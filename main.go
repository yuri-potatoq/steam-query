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
	"math"
	"net/http"
	"os"
	"os/signal"
	"path"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Eyevinn/hls-m3u8/m3u8"
	"golang.org/x/sync/errgroup"
)

type SteamAppDetailsData struct {
	Data SteamAppDetails `json:"data"`
}

type SteamAppDetails struct {
	AppName  string        `json:"name"`
	Trailers []TrailerData `json:"movies"`
}

type TrailerData struct {
	HLSManifest string `json:"hls_h264"`
}

var (
	tmpAudioFile = "audio.m4s"
	tmpVideoFile = "video.m4s"

	steamApiURL = "https://store.steampowered.com/api/appdetails"

	gamePagePattern = regexp.MustCompile("^https://store.steampowered.com/app/([0-9]+)/(.*)")
)

var (
	gamePageUrl string
	outputDir   string
	steamAppID  string
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt,
		syscall.SIGINT,
		syscall.SIGABRT,
		syscall.SIGKILL,
		syscall.SIGTERM)

	defer func() {
		if err := recover(); err != nil {
			log.Printf("Recovered from panic: %v\nStack Trace:\n%s", err, debug.Stack())
		}

		fmt.Println("removing temporary files...")
		os.Remove(tmpAudioFile)
		os.Remove(tmpVideoFile)
		cancel()
	}()

	flag.StringVar(&gamePageUrl, "game-page", "", `url for steam game page.`)
	flag.StringVar(&outputDir, "output-dir", getEnvString("OUTPUT_DIR", "./"), `output directory of result file. (default: ./)`)
	flag.StringVar(&steamAppID, "app-id", "", `steam app ID of the page game. (default: empty)`)
	flag.Parse()

	if gamePageUrl == "" {
		gamePageUrl = getEnvString("GAME_PAGE", "")
	}

	if gamePageUrl != "" {
		appId, err := getSteamAppID(gamePageUrl)
		if err != nil {
			log.Fatal(err)
		}
		steamAppID = appId
	}

	if steamAppID == "" {
		fmt.Println("didn't find any game page URL or steam app ID")
		return
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return runApp(ctx, steamAppID)
	})
	if err := g.Wait(); err != nil {
		fmt.Printf("Unexpected error: %+v\n", err)
	}
}

func runApp(ctx context.Context, steamAppID string) error {
	fm, err := SetupFileManager(steamAppID, tmpVideoFile, tmpAudioFile)
	if err != nil {
		return fmt.Errorf("setup file manager: %w", err)
	}

	if err := fm.extractMasterPlaylists(ctx); err != nil {
		return fmt.Errorf("extract master playlists: %w", err)
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

	videoPl, err := chooseResolution(ctx, fm.videoPlaylists)
	if err != nil {
		return err
	}

	w, err := SetupWindowTable()
	if err != nil {
		log.Fatal(err)
	}
	defer w.Close()

	fm.win = w
	w.RefreshRoutine(ctx)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return fm.mergeAndWriteFile(ctx, fm.outputVideoFile, getFileNames(videoPl)...)
	})
	g.Go(func() error {
		return fm.mergeAndWriteFile(ctx, fm.outputAudioFile, getFileNames(fm.audioPlaylist)...)
	})
	if err := g.Wait(); err != nil {
		return fmt.Errorf("writing temp files: %w", err)
	}

	outputPath, err := validateOutputPath(outputDir)
	if err != nil {
		return fmt.Errorf("output path validation: %w", err)
	}

	if err := TransformMedia(tmpVideoFile, tmpAudioFile, outputPath); err != nil {
		return fmt.Errorf("transforming to output format: %w", err)
	}
	return nil
}

/**
 * Helper functions
 */

func chooseResolution(ctx context.Context, playlists []*videoPlaylist) (*m3u8.MediaPlaylist, error) {
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

	selectedIdx, err := getInputNumber(ctx, 1, len(playlists))
	if err != nil {
		return nil, err
	}
	return playlists[selectedIdx-1].playlist, nil
}

func getCursorPos() (row int, col int, err error) {
	fmt.Printf("\033[6n\r")
	// Expected format: ESC [ {row} ; {col} R
	_, err = fmt.Scanf("\033[%d;%dR", &row, &col)
	if err != nil {
		return 0, 0, err
	}
	return row, col, nil
}

func getSteamAppID(url string) (string, error) {
	matches := gamePagePattern.FindStringSubmatch(url)
	if len(matches) < 2 {
		return "", errors.New("didn't find any matches for page URL")
	}

	return matches[1], nil
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

// TODO: when using fmt.Scanf or it's variants, we'll alwasys block the IO
// and will not be able to handle SIGINT signals.
// Search for an approach which allow us to cancel the read IO operation and gracefully shutdown the CLI.
func getInputNumber(ctx context.Context, start, end int) (int, error) {
	var optionNumber int
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
			fmt.Print("> ")
			nArgs, err := fmt.Scanf("%d\n", &optionNumber)
			if err == nil && nArgs == 1 && optionNumber <= end && optionNumber >= start {
				return optionNumber, nil
			}
			fmt.Println("Invalid option! Try it again...")
		}
	}
}

func chooseVideoPlaylist(ctx context.Context, details SteamAppDetails) (TrailerData, error) {
	fmt.Println("Select which video from the page you with download:")
	for i, _ := range details.Trailers {
		fmt.Printf("[%d] %dº video\n", i+1, i+1)
	}

	selectedIdx, err := getInputNumber(ctx, 1, len(details.Trailers))
	if err != nil {
		return TrailerData{}, err
	}
	return details.Trailers[selectedIdx-1], nil
}

/**
 * Application engine.
 */

type videoPlaylist struct {
	resolution string
	playlist   *m3u8.MediaPlaylist
}

type Engine struct {
	steamAppId       string
	basePlaylistsUrl string
	// resolution => Media Playlist Metadata
	videoPlaylists  []*videoPlaylist
	audioPlaylist   *m3u8.MediaPlaylist
	outputVideoFile *os.File
	outputAudioFile *os.File
	httpClient      *http.Client
	win             *windowTable
}

func SetupFileManager(steamAppId string, videoOutputFile, audioOutputFile string) (*Engine, error) {
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
		Timeout: time.Minute * 1,
	}

	return &Engine{
		steamAppId:      steamAppId,
		outputVideoFile: vF,
		outputAudioFile: aF,
		httpClient:      c,
	}, nil
}

func (e *Engine) mergeAndWriteFile(ctx context.Context, f io.WriteCloser, fileNames ...string) error {
	defer f.Close()

	progress := NewProgressLine()
	_, err := e.win.addLine(progress.Blocks()...)
	if err != nil {
		return err
	}

	// rounding to above them we will always have bar completed
	// TODO: think another way to have the progress bar and total of items synced
	stepPercentage := int(math.Ceil((1.0 / float64(len(fileNames))) * 100.0))

	for _, name := range fileNames {
		progress.UpdateInfo(fmt.Sprintf("Downloading %s", name))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s", e.basePlaylistsUrl, name), nil)
		if err != nil {
			return err
		}

		resp, err := e.httpClient.Do(req)
		if err != nil {
			return err
		}

		_, err = io.Copy(f, resp.Body)
		if err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()

		progress.Progress(stepPercentage)
	}
	return nil
}

func (e *Engine) extractMasterPlaylists(ctx context.Context) error {
	masterpl, err := e.selectMasterPlaylist(ctx)
	if err != nil {
		return err
	}

	// setup video playlist variants by resolution
	for _, variant := range masterpl.Variants {
		if pl, err := e.downloadAndDecodeM3U8File(ctx, variant.URI); err == nil {
			e.videoPlaylists = append(e.videoPlaylists, &videoPlaylist{
				resolution: variant.Resolution,
				playlist:   pl.(*m3u8.MediaPlaylist),
			})
		} else {
			return err
		}
	}

	for _, alt := range masterpl.GetAllAlternatives() {
		if alt.Type == "AUDIO" {
			if pl, err := e.downloadAndDecodeM3U8File(ctx, alt.URI); err == nil {
				e.audioPlaylist = pl.(*m3u8.MediaPlaylist)
				break
			} else {
				return err
			}
		}
	}

	return nil
}

func (e *Engine) selectMasterPlaylist(ctx context.Context) (*m3u8.MasterPlaylist, error) {
	appDetails, err := e.getSteamAppDetails(ctx)
	if err != nil {
		return nil, err
	}

	selectedTrailer, err := chooseVideoPlaylist(ctx, appDetails)
	if err != nil {
		return nil, err
	}

	// extract base url and master playlist name
	// https://host/path/to/app/hls_264_master.m3u8?t=1733940241
	lastSlashIdx := strings.LastIndex(selectedTrailer.HLSManifest, "/")
	questionMarkIndex := strings.Index(selectedTrailer.HLSManifest, "?")

	e.basePlaylistsUrl = selectedTrailer.HLSManifest[:lastSlashIdx]

	playlist, err := e.downloadAndDecodeM3U8File(ctx, selectedTrailer.HLSManifest[lastSlashIdx+1:questionMarkIndex])
	if err != nil {
		return nil, err
	}

	return playlist.(*m3u8.MasterPlaylist), nil
}

func (e *Engine) downloadAndDecodeM3U8File(ctx context.Context, fileName string) (m3u8.Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s", e.basePlaylistsUrl, fileName), nil)
	if err != nil {
		return nil, err
	}

	resp, err := e.httpClient.Do(req)
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

func (e *Engine) getSteamAppDetails(ctx context.Context) (SteamAppDetails, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?appids=%s", steamApiURL, steamAppID), nil)
	if err != nil {
		return SteamAppDetails{}, err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return SteamAppDetails{}, err
	}

	defer resp.Body.Close()
	var wrapper map[string]SteamAppDetailsData

	err = json.NewDecoder(resp.Body).Decode(&wrapper)
	if err != nil {
		return SteamAppDetails{}, err
	}

	data, ok := wrapper[steamAppID]
	if !ok {
		return SteamAppDetails{}, errors.New("can't find app details from steam API")
	}

	return data.Data, nil
}
