// Package ytupload uploads a video to YouTube using the Data API v3
// resumable upload protocol, reporting progress and ETA as it goes, then
// waits briefly for YouTube to finish initial processing. Video language
// (snippet.defaultLanguage / defaultAudioLanguage) is set as part of the
// initial insert when provided, since that's what influences the accuracy
// of YouTube's own automatic captions; there is no separate API call that
// forces captions to appear.
package ytupload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// insertURL, listURL, and thumbnailSetURL are vars (not consts) so tests
// can point them at an httptest.Server instead of the real YouTube API.
var (
	insertURL       = "https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status"
	listURL         = "https://www.googleapis.com/youtube/v3/videos?part=status,processingDetails&id="
	thumbnailSetURL = "https://www.googleapis.com/upload/youtube/v3/thumbnails/set?videoId="
)

const chunkSize = 8 << 20 // 8 MiB

// Options configures an upload.
type Options struct {
	AccessToken string
	Title       string
	Description string
	Tags        []string
	Privacy     string // public | unlisted | private
	Language    string // BCP-47, e.g. "en", "en-IN"; empty to skip

	// ThumbnailPath, if set, is uploaded as the video's custom thumbnail
	// right after the video ID is known. Setting a custom thumbnail via
	// the API requires the channel to have phone verification enabled;
	// if that's missing, YouTube's own error names it, and that raw
	// error text is surfaced rather than a generic wrapped one.
	ThumbnailPath string

	// NoWait skips polling for YouTube's initial processing status after
	// the upload completes.
	NoWait bool

	// Progress, if set, is called after every chunk with the fraction
	// uploaded (0..1) and an ETA. Progress may be nil.
	Progress func(fraction float64, eta time.Duration)

	// OnUploaded, if set, is called as soon as the video ID/URL are known,
	// i.e. right after the upload itself finishes and before any
	// processing-status wait. The link is valid to share immediately;
	// it does not need to wait for YouTube to finish processing.
	OnUploaded func(result *Result)
}

// Result is returned after a successful upload.
type Result struct {
	VideoID string
	URL     string
}

type videoResource struct {
	Snippet struct {
		Title                string   `json:"title"`
		Description          string   `json:"description"`
		Tags                 []string `json:"tags,omitempty"`
		DefaultLanguage      string   `json:"defaultLanguage,omitempty"`
		DefaultAudioLanguage string   `json:"defaultAudioLanguage,omitempty"`
	} `json:"snippet"`
	Status struct {
		PrivacyStatus string `json:"privacyStatus"`
	} `json:"status"`
}

type insertedVideo struct {
	ID string `json:"id"`
}

// Upload sends path to YouTube via the resumable upload protocol and
// returns the resulting video ID/link.
func Upload(path string, opts Options) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening video: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat video: %w", err)
	}
	size := info.Size()

	sessionURI, err := startSession(opts, size)
	if err != nil {
		return nil, err
	}

	video, err := uploadChunks(sessionURI, f, size, opts)
	if err != nil {
		return nil, err
	}

	result := &Result{
		VideoID: video.ID,
		URL:     "https://youtu.be/" + video.ID,
	}
	if opts.OnUploaded != nil {
		opts.OnUploaded(result)
	}

	if opts.ThumbnailPath != "" {
		if err := setThumbnail(opts.AccessToken, video.ID, opts.ThumbnailPath); err != nil {
			return result, fmt.Errorf("video uploaded (%s) but setting the thumbnail failed: %w", result.URL, err)
		}
	}

	if !opts.NoWait {
		waitForProcessing(opts.AccessToken, video.ID)
	}

	return result, nil
}

func startSession(opts Options, size int64) (string, error) {
	body := videoResource{}
	body.Snippet.Title = opts.Title
	body.Snippet.Description = opts.Description
	body.Snippet.Tags = opts.Tags
	body.Snippet.DefaultLanguage = opts.Language
	body.Snippet.DefaultAudioLanguage = opts.Language
	body.Status.PrivacyStatus = opts.Privacy

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encoding video metadata: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, insertURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+opts.AccessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", "video/mp4")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("starting upload session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("starting upload session: %s: %s", resp.Status, readErrBody(resp))
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("upload session response had no Location header")
	}
	return loc, nil
}

func uploadChunks(sessionURI string, f *os.File, size int64, opts Options) (*insertedVideo, error) {
	buf := make([]byte, chunkSize)
	var sent int64
	start := time.Now()

	for sent < size {
		n, err := io.ReadFull(f, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return nil, fmt.Errorf("reading video chunk: %w", err)
		}
		chunk := buf[:n]
		end := sent + int64(n) - 1

		req, err := http.NewRequest(http.MethodPut, sessionURI, bytes.NewReader(chunk))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", sent, end, size))
		req.Header.Set("Content-Type", "video/mp4")
		req.ContentLength = int64(n)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("uploading chunk: %w", err)
		}
		sent += int64(n)

		if resp.StatusCode == 200 || resp.StatusCode == 201 {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return nil, fmt.Errorf("reading upload response: %w", readErr)
			}
			if opts.Progress != nil {
				opts.Progress(1.0, 0)
			}
			var video insertedVideo
			if err := json.Unmarshal(body, &video); err != nil {
				return nil, fmt.Errorf("parsing upload response: %w", err)
			}
			return &video, nil
		}
		if resp.StatusCode != 308 {
			msg := readErrBody(resp)
			resp.Body.Close()
			return nil, fmt.Errorf("uploading chunk: %s: %s", resp.Status, msg)
		}
		resp.Body.Close()

		if opts.Progress != nil {
			elapsed := time.Since(start).Seconds()
			var eta time.Duration
			if elapsed > 0 {
				rate := float64(sent) / elapsed
				if rate > 0 {
					remaining := float64(size - sent)
					eta = time.Duration(remaining/rate) * time.Second
				}
			}
			opts.Progress(float64(sent)/float64(size), eta)
		}
	}

	return nil, fmt.Errorf("upload ended without a final response")
}

// setThumbnail uploads the PNG at path as videoID's custom thumbnail.
// This requires the channel to have phone verification enabled; when
// that's missing, YouTube's API error names it directly, so the raw
// response body is surfaced rather than a generic wrapped message.
func setThumbnail(accessToken, videoID, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading thumbnail: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, thumbnailSetURL+videoID, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "image/png")
	req.ContentLength = int64(len(data))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("setting thumbnail: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("setting thumbnail: %s: %s", resp.Status, readErrBody(resp))
	}
	return nil
}

// waitForProcessing polls YouTube for up to ~2 minutes so the caller can
// report when initial processing finishes. It never returns an error;
// failures or timeouts just mean the caller prints the link without a
// processed confirmation.
func waitForProcessing(accessToken, videoID string) {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		status, err := fetchProcessingStatus(accessToken, videoID)
		if err == nil {
			fmt.Println("==> processing status:", status)
			if status == "succeeded" || status == "failed" || status == "terminated" {
				return
			}
		}
		time.Sleep(5 * time.Second)
	}
}

func fetchProcessingStatus(accessToken, videoID string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, listURL+videoID, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %s", resp.Status)
	}

	var out struct {
		Items []struct {
			ProcessingDetails struct {
				ProcessingStatus string `json:"processingStatus"`
			} `json:"processingDetails"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Items) == 0 {
		return "", fmt.Errorf("video not found")
	}
	return out.Items[0].ProcessingDetails.ProcessingStatus, nil
}

func readErrBody(resp *http.Response) string {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "(could not read error body)"
	}
	return string(body)
}
