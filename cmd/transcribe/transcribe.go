package transcribe

import (
	"context"
	"dgram/lib/config"
	"dgram/lib/fsys"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/andrerfcsantos/deepgram-go-captions/converters"
	"github.com/andrerfcsantos/deepgram-go-captions/renderers"
	api "github.com/deepgram/deepgram-go-sdk/pkg/api/listen/v1/rest"
	interfacesv1 "github.com/deepgram/deepgram-go-sdk/pkg/api/listen/v1/rest/interfaces"
	interfaces "github.com/deepgram/deepgram-go-sdk/pkg/client/interfaces"
	client "github.com/deepgram/deepgram-go-sdk/pkg/client/listen"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/spf13/cobra"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

var (
	cfg *config.Config
)

const (
	audioDirectory         = ".audio"
	transcriptionDirectory = ".transcriptions"
	graphsDirectory        = ".graphs"
)

func filesFromGlobs(globs []string) ([]string, error) {
	files := make([]string, 0, 4)
	for _, glob := range globs {
		matches, err := filepath.Glob(glob)
		if err != nil {
			return nil, fmt.Errorf("globbing files with glob %q: %w", glob, err)
		}
		files = append(files, matches...)
	}
	return files, nil
}

func getDgClient(apiKey string) (*api.Client, error) {
	client.Init(client.InitLib{
		LogLevel: client.LogLevelStandard, // LogLevelStandard / LogLevelFull / LogLevelTrace / LogLevelVerbose
	})

	// create a Deepgram client
	c := client.NewREST(apiKey, &interfaces.ClientOptions{
		APIKey: apiKey,
	})
	dg := api.New(c)

	return dg, nil
}

var AudioExtensions = []string{".wav", ".mp3", ".m4a", ".flac", ".ogg", ".opus", ".webm", ".aac", ".wma", ".aiff", ".aif", ".aifc", ".caf", ".amr", ".au", ".snd", ".gsm", ".m4r", ".3gp", ".3g2", ".aa", ".aax", ".act", ".aup", ".awb", ".dct", ".dss", ".dvf", ".flac", ".gsm", ".ivs", ".m4a", ".m4b", ".m4p", ".mmf", ".mpc", ".msv", ".nmf", ".nsf", ".ogg", ".oga", ".mogg", ".opus", ".ra", ".rm", ".raw", ".sln", ".tta", ".vox", ".wav", ".wma", ".wv", ".webm", ".8svx", ".cda"}
var VideoExtensions = []string{".mp4", ".mov", ".avi", ".mkv", ".flv", ".wmv", ".webm", ".m4v", ".3gp", ".3g2", ".asf"}

type FilePath string

func (f FilePath) Dir() string {
	return filepath.Dir(string(f))
}

func (f FilePath) Name() string {
	return filepath.Base(string(f))
}

func (f FilePath) Ext() string {
	return filepath.Ext(string(f))
}

func (f FilePath) Base() string {
	return strings.TrimSuffix(f.Name(), f.Ext())
}

func (f FilePath) Exists() bool {
	return fsys.FileExists(string(f))
}

func audioForFile(file FilePath) (FilePath, error) {
	isVideo := slices.Contains(VideoExtensions, file.Ext())
	if isVideo {
		dir := filepath.Join(file.Dir(), audioDirectory)
		for _, ext := range AudioExtensions {
			audioFile := FilePath(filepath.Join(dir, file.Base()+ext))
			if audioFile.Exists() {
				return audioFile, nil
			}
		}

		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return "", fmt.Errorf("creating audio directory %q: %w", dir, err)
		}

		audioPath := FilePath(filepath.Join(dir, file.Base()+".mp3"))

		fmt.Printf("Converting %q to %q\n", file, audioPath)
		err = ffmpeg.
			Input(string(file)).
			Output(string(audioPath)).
			OverWriteOutput().
			Silent(true).
			Run()

		if err != nil {
			return "", fmt.Errorf("running ffmpeg converting %q to %q: %w", file, audioPath, err)
		}

		return audioPath, nil
	}

	isAudio := slices.Contains(AudioExtensions, file.Ext())
	if isAudio {
		return file, nil
	}

	return "", fmt.Errorf("file %q is not a supported audio or video file", file)
}

func ProcessFile(dg *api.Client, file FilePath) (*interfacesv1.PreRecordedResponse, error) {

	transcriptDir := filepath.Join(file.Dir(), transcriptionDirectory)
	transcript := FilePath(filepath.Join(transcriptDir, file.Base()+"_response.json"))
	if transcript.Exists() {
		fmt.Printf("Transcript file %q already exists, using it\n", transcript)
		var r interfacesv1.PreRecordedResponse
		fileData, err := os.ReadFile(string(transcript))
		if err != nil {
			return nil, fmt.Errorf("reading existing transcript file %q: %w", transcript, err)
		}
		err = json.Unmarshal(fileData, &r)
		if err != nil {
			return nil, fmt.Errorf("unmarshaling existing transcript file %q: %w", transcript, err)
		}
		return &r, nil
	}

	isVideo := slices.Contains(VideoExtensions, file.Ext())
	isAudio := slices.Contains(AudioExtensions, file.Ext())

	if !isVideo && !isAudio {
		fmt.Printf("File %q is not a supported audio or video file, skipping\n", file)
		return nil, nil
	}

	audioFile, err := audioForFile(file)
	if err != nil {
		return nil, fmt.Errorf("getting audio file for %q: %w", file, err)
	}

	// Go context
	ctx := context.Background()

	// set the Transcription options
	options := &interfaces.PreRecordedTranscriptionOptions{
		Model:       "nova-2",
		Punctuate:   true,
		Paragraphs:  true,
		SmartFormat: true,
		Language:    "en-US",
		Diarize:     true,
		Utterances:  true,
	}

	fmt.Printf("Transcribing %q\n", file)
	res, err := dg.FromFile(ctx, string(audioFile), options)
	if err != nil {
		if e, ok := err.(*interfaces.StatusError); ok {
			return nil, fmt.Errorf("deepgram status error (%s) %s ", e.DeepgramError.ErrCode, e.DeepgramError.ErrMsg)
		}
		return nil, fmt.Errorf("getting response from deepgram: %w", err)
	}

	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling file response: %w", err)
	}

	err = os.MkdirAll(transcriptDir, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("creating transcript directory %q: %w", transcriptDir, err)
	}

	err = os.WriteFile(string(transcript), data, 0644)
	if err != nil {
		return nil, fmt.Errorf("writing transcript file %q: %w", transcript, err)
	}

	fmt.Printf("Transcript saved to %q\n", transcript)

	return res, nil
}

var transcribeCmd = &cobra.Command{
	Use:   "transcribe",
	Short: "transcribe video and audio files",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {

		files, err := filesFromGlobs(args)
		if err != nil {
			return fmt.Errorf("getting file paths: %w", err)
		}

		dg, err := getDgClient(cfg.GetString("apikey"))
		if err != nil {
			return fmt.Errorf("creating deepgram client: %w", err)
		}

		type FileResult struct {
			File string  `json:"file"`
			WPM  float64 `json:"wpm"`
		}

		type JobResult struct {
			FileResult FileResult
			Error      error
		}

		const maxWorkers = 4
		jobs := make(chan string, len(files))
		results := make(chan JobResult, len(files))

		var wg sync.WaitGroup

		// Start worker goroutines
		for i := 0; i < maxWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for file := range jobs {
					fp := FilePath(file)
					r, err := ProcessFile(dg, fp)
					if err != nil {
						results <- JobResult{Error: fmt.Errorf("processing file %q: %w", file, err)}
						continue
					}

					err = CreateGraph(r, fp)
					if err != nil {
						results <- JobResult{Error: fmt.Errorf("creating graph: %w", err)}
						continue
					}

					srtPath := filepath.Join(fp.Dir(), fp.Base()+".srt")

					if !fsys.FileExists(srtPath) {
						conv := converters.NewDeepgramConverter(r)
						srt, err := renderers.SRT(conv)
						if err != nil {
							results <- JobResult{Error: fmt.Errorf("rendering SRT for %s: %w", file, err)}
							continue
						}

						err = os.WriteFile(srtPath, []byte(srt), 0644)
						if err != nil {
							results <- JobResult{Error: fmt.Errorf("writing SRT file %q: %w", srtPath, err)}
							continue
						}
					} else {
						fmt.Printf("SRT file %q already exists, skipping\n", srtPath)
					}

					nWords := 0
					for _, c := range r.Results.Channels {
						nWords += len(c.Alternatives[0].Words)
					}

					wpm := float64(nWords) / (r.Metadata.Duration / 60)
					results <- JobResult{FileResult: FileResult{File: file, WPM: wpm}}
				}
			}()
		}

		// Send jobs to workers
		go func() {
			defer close(jobs)
			for _, file := range files {
				jobs <- file
			}
		}()

		// Wait for all workers to finish
		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect results
		errors := make([]JobResult, 0)
		wpms := make([]FileResult, 0, len(files))
		for result := range results {
			if result.Error != nil {
				errors = append(errors, result)
				continue
			}
			wpms = append(wpms, result.FileResult)
		}

		slices.SortFunc(wpms, func(a, b FileResult) int {
			if a.WPM < b.WPM {
				return 1
			}
			if a.WPM > b.WPM {
				return -1
			}

			return 0
		})

		wpms_json, err := json.MarshalIndent(wpms, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling wpms: %w", err)
		}

		err = os.WriteFile("wpms.json", wpms_json, 0644)
		if err != nil {
			return fmt.Errorf("writing wpms.json: %w", err)
		}

		if len(errors) > 0 {
			fmt.Println("Some errors occurred during processing these files:")
			for _, e := range errors {
				fmt.Printf("  - %v (%v)\n", e.FileResult.File, e.Error)
			}
		}
		return nil
	},
}

func generateWordCountSeries(r *interfacesv1.PreRecordedResponse) []opts.BarData {
	mins := int(math.Trunc(r.Metadata.Duration/60) + 1)
	counts := make([]int, mins)

	for _, c := range r.Results.Channels {
		for _, w := range c.Alternatives[0].Words {
			minute := int(w.Start / 60)
			counts[minute]++
		}
	}

	items := make([]opts.BarData, mins)
	for i, c := range counts {
		items[i] = opts.BarData{Value: c}
	}

	return items
}

func generateMinutesSeries(r *interfacesv1.PreRecordedResponse) []int {
	mins := int(math.Trunc(r.Metadata.Duration/60) + 1)
	items := make([]int, 0)
	for i := 0; i < mins; i++ {
		items = append(items, i)
	}
	return items
}

func CreateGraph(r *interfacesv1.PreRecordedResponse, file FilePath) error {

	bar := charts.NewBar()
	bar.SetGlobalOptions(charts.WithTitleOpts(opts.Title{
		Title: string(file),
	}))

	bar.SetXAxis(generateMinutesSeries(r)).
		AddSeries("Words", generateWordCountSeries(r))

	dir := filepath.Join(file.Dir(), graphsDirectory)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("creating graphs directory: %w", err)
	}

	f, err := os.Create(filepath.Join(dir, file.Base()+"_graph.html"))
	if err != nil {
		return fmt.Errorf("creating graph file: %w", err)
	}
	err = bar.Render(f)
	if err != nil {
		return fmt.Errorf("rendering graph: %w", err)
	}
	return nil
}

func GetCmd(config *config.Config) *cobra.Command {
	cfg = config

	return transcribeCmd
}
