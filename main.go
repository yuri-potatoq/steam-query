package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Eyevinn/hls-m3u8/m3u8"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type DataProps struct {
	HLSManifest string `json:"hlsManifest"`
}

var (
	tmpAudioFile = "audio.m4s"
	tmpVideoFile = "video.m4s"
)

func main() {
	gamePageUrl := flag.String("game-page", "", `url for steam game page`)
	flag.Parse()

	defer func() {
		os.Remove(tmpAudioFile)
		os.Remove(tmpVideoFile)
	}()

	fm, err := SetupFileManager(*gamePageUrl, tmpVideoFile, tmpAudioFile)
	if err != nil {
		log.Fatal(err)
	}

	err = fm.ExtractMasterPlaylists()
	if err != nil {
		log.Fatal(err)
	}

	getFileNames := func(m *m3u8.MediaPlaylist) []string {
		fileNames := make([]string, 0)
		fileNames = append(fileNames, m.Map.URI)
		for _, seg := range m.Segments {
			if seg != nil {
				fileNames = append(fileNames, seg.URI)
			}
		}
		return fileNames
	}

	err = fm.mergeAndWriteFile(fm.outputVideoFile, getFileNames(fm.videoPlaylists["640x360"])...)
	if err != nil {
		log.Fatal(err)
	}

	err = fm.mergeAndWriteFile(fm.outputAudioFile, getFileNames(fm.audioPlaylist)...)
	if err != nil {
		log.Fatal(err)
	}

	if err := TransformMedia(tmpVideoFile, tmpAudioFile, "./output.mp4"); err != nil {
		log.Fatal(err)
	}

}

type FileManager struct {
	gamePageUrl      string
	basePlaylistsUrl string
	// resolution => Media Playlist Metadata
	videoPlaylists  map[string]*m3u8.MediaPlaylist
	audioPlaylist   *m3u8.MediaPlaylist
	outputVideoFile *os.File
	outputAudioFile *os.File
}

func SetupFileManager(gamePageUrl, videoOutputFile, audioOutputFile string) (*FileManager, error) {
	vF, err := os.OpenFile(videoOutputFile, os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		return nil, err
	}

	aF, err := os.OpenFile(audioOutputFile, os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		return nil, err
	}

	return &FileManager{gamePageUrl: gamePageUrl, outputVideoFile: vF, outputAudioFile: aF, videoPlaylists: map[string]*m3u8.MediaPlaylist{}}, nil
}

func (fm *FileManager) mergeAndWriteFile(f *os.File, fileNames ...string) error {
	defer f.Close()

	writeToBin := func(part string) error {
		resp, err := http.Get(fmt.Sprintf("%s/%s", fm.basePlaylistsUrl, part))
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		n, err := io.Copy(f, resp.Body)
		if err != nil {
			return err
		}

		fmt.Printf("written %d bytes for %s into %s\n", n, part, f.Name())
		return nil
	}

	for _, name := range fileNames {
		if err := writeToBin(name); err != nil {
			return err
		}
	}
	return nil
}

func (fm *FileManager) ExtractMasterPlaylists() error {
	html, err := downloadGamePageDocument(fm.gamePageUrl)
	if err != nil {
		return err
	}

	data, err := extractDataProps(html)
	if err != nil {
		return err
	}

	// extract base url and master playlist name
	// https://host/path/to/app/hls_264_master.m3u8?t=1733940241
	lastSlashIdx := strings.LastIndex(data[0].HLSManifest, "/")
	questionMarkIndex := strings.Index(data[0].HLSManifest, "?")
	fm.basePlaylistsUrl = data[0].HLSManifest[:lastSlashIdx]

	//FIXME: for test, only use the first video of the page
	playlist, err := fm.downloadAndDecodeM3U8File(data[0].HLSManifest[lastSlashIdx+1 : questionMarkIndex])
	if err != nil {
		return err
	}

	masterpl := playlist.(*m3u8.MasterPlaylist)
	// setup video playlist variants by resolution
	for _, variant := range masterpl.Variants {
		fmt.Printf("Variant: %+v\n\n", variant.URI)
		if pl, err := fm.downloadAndDecodeM3U8File(variant.URI); err == nil {
			fm.videoPlaylists[variant.Resolution] = pl.(*m3u8.MediaPlaylist)
		} else {
			return err
		}
	}

	for _, alt := range masterpl.GetAllAlternatives() {
		if alt.Type == "AUDIO" {
			if pl, err := fm.downloadAndDecodeM3U8File(alt.URI); err == nil {
				fm.audioPlaylist = pl.(*m3u8.MediaPlaylist)
				fmt.Printf("Audio: %+v\n\n", fm.audioPlaylist)
				break
			} else {
				return err
			}
		}
	}

	return nil
}

// https://video.fastly.steamstatic.com/store_trailers/1063730/798628/39a388c693c9f2d892cfad5d95ab25dd759662b5/1750611698/hls_264_master.m3u8
// https://video.fastly.steamstatic.com/store_trailers/1063730/798628/39a388c693c9f2d892cfad5d95ab25dd759662b5/1750611698/hls_264_3_video.m3u8
func (fm *FileManager) downloadAndDecodeM3U8File(fileName string) (m3u8.Playlist, error) {
	resp, err := http.Get(fmt.Sprintf("%s/%s", fm.basePlaylistsUrl, fileName))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	paylist, _, err := m3u8.DecodeFrom(resp.Body, false)
	if err != nil {
		return nil, err
	}
	return paylist, nil
}

func downloadGamePageDocument(url string) (*html.Node, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	node, err := html.Parse(res.Body)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func extractDataProps(n *html.Node) ([]DataProps, error) {
	var props []DataProps

	for d := range n.Descendants() {
		if d.DataAtom == atom.Div && len(d.Attr) > 1 {
			_, is_player_div := ExtractAttribute(d.Attr, func(t html.Attribute) bool {
				return t.Key == "class" && t.Val == "highlight_player_item highlight_movie"
			})
			props_attrs, has_props := ExtractAttribute(d.Attr, func(t html.Attribute) bool {
				return t.Key == "data-props"
			})

			if is_player_div && has_props {
				var p DataProps
				err := json.Unmarshal([]byte(props_attrs.Val), &p)
				if err != nil {
					return nil, err
				}

				props = append(props, p)
			}
		}
	}

	return props, nil
}

func ExtractAttribute(xs []html.Attribute, f func(html.Attribute) bool) (html.Attribute, bool) {
	for _, it := range xs {
		if f(it) {
			return it, true
		}
	}
	return html.Attribute{}, false
}
