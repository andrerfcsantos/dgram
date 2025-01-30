package transcribe

import (
	"context"
	"dgram/lib/config"
	"dgram/lib/fsys"
	"encoding/json"
	"fmt"
	api "github.com/deepgram/deepgram-go-sdk/pkg/api/listen/v1/rest"
	interfacesv1 "github.com/deepgram/deepgram-go-sdk/pkg/api/prerecorded/v1/interfaces"
	interfaces "github.com/deepgram/deepgram-go-sdk/pkg/client/interfaces"
	client "github.com/deepgram/deepgram-go-sdk/pkg/client/listen"
	"github.com/spf13/cobra"
	ffmpeg "github.com/u2takey/ffmpeg-go"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var (
	cfg *config.Config
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
		for _, ext := range AudioExtensions {
			audioFile := FilePath(filepath.Join(file.Dir(), file.Base()+ext))
			if audioFile.Exists() {
				return audioFile, nil
			}
		}

		audioPath := FilePath(filepath.Join(file.Dir(), file.Base()+".mp3"))

		fmt.Printf("Converting %q to %q\n", file, audioPath)
		err := ffmpeg.Input(string(file)).Output(string(audioPath)).OverWriteOutput().ErrorToStdOut().Run()
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

	transcript := FilePath(filepath.Join(file.Dir(), file.Base()+"_response.json"))
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

	err = os.WriteFile(string(transcript), data, 0644)
	if err != nil {
		return nil, fmt.Errorf("writing transcript file %q: %w", transcript, err)
	}

	fmt.Printf("Transcript saved to %q\n", transcript)

	return nil, nil
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

		type FileResult struct {
			File string
			WPM  float64
		}

		wpms := make([]FileResult, 0, len(files))

		for _, file := range files {
			fp := FilePath(file)
			r, err := ProcessFile(dg, fp)
			if err != nil {
				return fmt.Errorf("processing file %q: %w", file, err)
			}

			srt, err := r.ToSRT()
			if err != nil {
				return fmt.Errorf("converting response to SRT: %w", err)
			}

			err = os.WriteFile(fp.Base()+".srt", []byte(srt), 0644)
			if err != nil {
				return fmt.Errorf("writing SRT file: %w", err)
			}

			nWords := 0
			for _, c := range r.Results.Channels {
				nWords += len(c.Alternatives[0].Words)
			}

			wpms = append(wpms, FileResult{File: file, WPM: float64(nWords) / (r.Metadata.Duration / 60)})
		}

		slices.SortFunc(wpms, func(a, b FileResult) int {
			if a.WPM < b.WPM {
				return -1
			}
			if a.WPM > b.WPM {
				return 1
			}

			return 0
		})

		for _, r := range wpms {
			fmt.Printf("%s: %.2f WPM\n", r.File, r.WPM)
		}

		return nil
	},
}

func GetCmd(config *config.Config) *cobra.Command {
	cfg = config

	return transcribeCmd
}
