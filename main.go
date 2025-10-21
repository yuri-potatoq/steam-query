package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Eyevinn/hls-m3u8/m3u8"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/sync/errgroup"
)

type DataProps struct {
	HLSManifest string `json:"hlsManifest"`
}

var (
	tmpAudioFile = "audio.m4s"
	tmpVideoFile = "video.m4s"
)

func getEnvString(name string, dft ...string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		if len(dft) >= 1 {
			return dft[0]
		}
		log.Fatalf("can't find environment variable for [%s]", name)
	}
	return value
}

func chooseResolution(playlists []*videoPlaylist) *m3u8.MediaPlaylist {
	fmt.Println("Select output resolution option:")

	// sort playlist by resolution
	slices.SortFunc(playlists, func(a, b *videoPlaylist) int {
		aNumber, _ := strconv.Atoi(strings.SplitN(a.resolution, "x", 1)[0])
		bNumber, _ := strconv.Atoi(strings.SplitN(b.resolution, "x", 1)[0])

		return cmp.Compare(aNumber, bNumber)
	})
	for i, pl := range playlists {
		fmt.Printf(" [%d] %s\n", i+1, pl.resolution)
	}

	return playlists[getInputNumber(1, len(playlists))-1].playlist
}

func getInputNumber(start, end int) int {
	var optionNumber int

	for {
		fmt.Print("> ")
		nArgs, err := fmt.Scanf("%d\n", &optionNumber)
		if err == nil && nArgs == 1 && optionNumber <= end && optionNumber >= start {
			return optionNumber
		}
		fmt.Println("Invalid option! Try it again...")
	}
}

// func ttySize() int {
//  _, _, errno := syscall.Syscall(
//         syscall.SYS_IOCTL,
//         uintptr(f.Fd()),
//         uintptr(_I2C_RDWR),
//         uintptr(unsafe.Pointer(&data)),
//     )
//     if (errno != 0) {
//         err = errno
//     }
// }

// TODO:
// use slog package instead
// handle all fatal errors to user friendly messages
func main() {
	defer func() {
		//TODO: use graceful shutdown with os.Signal
		if err := recover(); err != nil {
			fmt.Printf("recovered error %v\n", err)
		}

		fmt.Println("removing temporary files...")
		os.Remove(tmpAudioFile)
		os.Remove(tmpVideoFile)
	}()

	gamePageUrl := flag.String("game-page", "", `url for steam game page.`)
	outputDir := flag.String("output-dir", getEnvString("OUTPUT_DIR", "./"), `output directory of result file. (default: ./)`)
	flag.Parse()

	if *gamePageUrl == "" {
		*gamePageUrl = getEnvString("GAME_PAGE")
	}

	fm, err := SetupFileManager(*gamePageUrl, tmpVideoFile, tmpAudioFile)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	defer cancel()

	if err := fm.ExtractMasterPlaylists(ctx); err != nil {
		log.Fatal(err)
	}

	//TODO: make generic helper to map new files
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

	videoPl := chooseResolution(fm.videoPlaylists)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return fm.mergeAndWriteFile(ctx, fm.outputVideoFile, getFileNames(videoPl)...)
	})
	g.Go(func() error {
		return fm.mergeAndWriteFile(ctx, fm.outputAudioFile, getFileNames(fm.audioPlaylist)...)
	})
	g.Wait()

	outputPath, err := validateOutputPath(*outputDir)
	if err != nil {
		log.Fatal(err)
	}

	if err := TransformMedia(tmpVideoFile, tmpAudioFile, outputPath); err != nil {
		log.Fatal(err)
	}
}

func validateOutputPath(outPath string) (string, error) {
	outPath = path.Clean(outPath)
	info, err := os.Stat(outPath)
	if err != nil {
		return "", err
	}

	if !info.IsDir() {
		return "", errors.New("can't use provide directory")
	}

	return path.Join(outPath, "output.mp4"), nil
}

type videoPlaylist struct {
	resolution string
	playlist   *m3u8.MediaPlaylist
}

type FileManager struct {
	gamePageUrl      string
	basePlaylistsUrl string
	// resolution => Media Playlist Metadata
	videoPlaylists  []*videoPlaylist
	audioPlaylist   *m3u8.MediaPlaylist
	outputVideoFile *os.File
	outputAudioFile *os.File
	httpClient      *http.Client
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

	c := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     10,
			IdleConnTimeout:     time.Second * 10,
		},
		Timeout: time.Second * 5,
	}

	return &FileManager{
		gamePageUrl:     gamePageUrl,
		outputVideoFile: vF,
		outputAudioFile: aF,
		httpClient:      c,
	}, nil
}

func (fm *FileManager) mergeAndWriteFile(ctx context.Context, f io.WriteCloser, fileNames ...string) error {
	defer f.Close()

	for _, name := range fileNames {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s", fm.basePlaylistsUrl, name), nil)
		if err != nil {
			return err
		}

		resp, err := fm.httpClient.Do(req)
		if err != nil {
			return err
		}

		n, err := io.Copy(f, resp.Body)
		if err != nil {
			return err
		}

		resp.Body.Close()
		fmt.Printf("written %d bytes for %s\n", n, name)
	}
	return nil
}

func (fm *FileManager) ExtractMasterPlaylists(ctx context.Context) error {
	masterpl, err := fm.selectMasterPlaylist(ctx)
	if err != nil {
		return err
	}

	// setup video playlist variants by resolution
	for _, variant := range masterpl.Variants {
		if pl, err := fm.downloadAndDecodeM3U8File(ctx, variant.URI); err == nil {
			fm.videoPlaylists = append(fm.videoPlaylists, &videoPlaylist{
				resolution: variant.Resolution,
				playlist:   pl.(*m3u8.MediaPlaylist),
			})
		} else {
			return err
		}
	}

	for _, alt := range masterpl.GetAllAlternatives() {
		if alt.Type == "AUDIO" {
			if pl, err := fm.downloadAndDecodeM3U8File(ctx, alt.URI); err == nil {
				fm.audioPlaylist = pl.(*m3u8.MediaPlaylist)
				break
			} else {
				return err
			}
		}
	}

	return nil
}

func (fm *FileManager) selectMasterPlaylist(ctx context.Context) (*m3u8.MasterPlaylist, error) {
	html, err := fm.downloadGamePageDocument(ctx, fm.gamePageUrl)
	if err != nil {
		return nil, err
	}

	data, err := extractDataProps(html)
	if err != nil {
		return nil, err
	}

	selectedVideoProps := chooseVideoPlaylist(data)

	// extract base url and master playlist name
	// https://host/path/to/app/hls_264_master.m3u8?t=1733940241
	lastSlashIdx := strings.LastIndex(selectedVideoProps.HLSManifest, "/")
	questionMarkIndex := strings.Index(selectedVideoProps.HLSManifest, "?")

	fm.basePlaylistsUrl = selectedVideoProps.HLSManifest[:lastSlashIdx]

	playlist, err := fm.downloadAndDecodeM3U8File(ctx, selectedVideoProps.HLSManifest[lastSlashIdx+1:questionMarkIndex])
	if err != nil {
		return nil, err
	}

	return playlist.(*m3u8.MasterPlaylist), nil
}

func chooseVideoPlaylist(options []DataProps) DataProps {
	fmt.Println("Select which video from the page you with download:")
	for i, _ := range options {
		fmt.Printf("[%d] %dº video\n", i+1, i+1)
	}
	return options[getInputNumber(1, len(options))-1]
}

func (fm *FileManager) downloadAndDecodeM3U8File(ctx context.Context, fileName string) (m3u8.Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s", fm.basePlaylistsUrl, fileName), nil)
	if err != nil {
		return nil, err
	}

	resp, err := fm.httpClient.Do(req)
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

func (fm *FileManager) downloadGamePageDocument(ctx context.Context, url string) (*html.Node, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := fm.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	node, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	return node, nil
}

// The extraction use depth-first preorder traversal, so the elements are stored
// in the some order as they are on HTML.
func extractDataProps(n *html.Node) ([]DataProps, error) {
	var props []DataProps

	for d := range n.Descendants() {
		if d.DataAtom == atom.Div && len(d.Attr) > 1 {
			_, is_player_div := extractAttribute(d.Attr, func(t html.Attribute) bool {
				return t.Key == "class" && t.Val == "highlight_player_item highlight_movie"
			})
			props_attrs, has_props := extractAttribute(d.Attr, func(t html.Attribute) bool {
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

func extractAttribute(xs []html.Attribute, f func(html.Attribute) bool) (html.Attribute, bool) {
	for _, it := range xs {
		if f(it) {
			return it, true
		}
	}
	return html.Attribute{}, false
}
