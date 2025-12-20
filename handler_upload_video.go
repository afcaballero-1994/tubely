package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxSize = 1 << 30
	var rder io.ReadCloser

	rder = http.MaxBytesReader(w, rder, maxSize)
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	viData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, 500, "Could not get video data", err)
		return
	}
	if viData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	f, h, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, 500, "could not get data from request", err)
		return
	}
	defer f.Close()
	mediaType, _, err := mime.ParseMediaType(h.Header.Get("Content-Type"))
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusNotAcceptable, "unsupported file format", nil)
		return
	}
	tmp, err := os.CreateTemp("", "tub-uploas.mp4")
	if err != nil {
		respondWithError(w, 500, "Could not create tmp file", err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	_, err = io.Copy(tmp, f)
	if err != nil {
		respondWithError(w, 500, "Could not copy video content", err)
		return
	}
	st, err := getVideoAspectRatio(tmp.Name())
	var prefix string
	if err != nil {
		respondWithError(w, 500, "could not get video data from file", err)
		return
	}

	proc, err := processVideoForStart(tmp.Name())
	if err != nil {
		respondWithError(w, 500, "could not process video", nil)
		return
	}
	

	switch st {
	case "16:9":
		prefix = "landscape/"
	case "9:16":
		prefix = "portrait/"
	default:
		prefix = "other/"
	}
	
	key := make([]byte, 32)
	rand.Read(key)
	filename := prefix + base64.RawURLEncoding.EncodeToString(key) + ".mp4"

	fproc, err := os.Open(proc)
	if err != nil {
		respondWithError(w, 500, "could not open processed file", err)
		return
	}
	defer fproc.Close()
	defer os.Remove(proc)

	tmp.Seek(0, io.SeekStart)
	cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &filename,
		Body:        fproc,
		ContentType: &mediaType,
	})
	viURL := cfg.s3CfDistribution + filename
	viData.VideoURL = &viURL
	
	err = cfg.db.UpdateVideo(viData)
	if err != nil {
		respondWithError(w, 500, "could not update video data", err)
		return
	}
	respondWithJSON(w, http.StatusOK, viData)
}

func getVideoAspectRatio(filePath string) (string, error) {
	result := exec.Command("ffprobe", "-v", "error",
		"-print_format", "json", "-show_streams", filePath)

	var b bytes.Buffer
	result.Stdout = &b

	err := result.Run()
	if err != nil {
		log.Printf("%v", err)
		return "", err
	}

	var viData VideoM
	if err := json.Unmarshal(b.Bytes(), &viData); err != nil {
		return "", err
	}

	width := viData.Streams[0].Width
	height := viData.Streams[0].Height
	
	if width == 16 * height / 9 {
		return "16:9", nil
	} else if height == 16 * width / 9 {
		return "9:16", nil
	} else {
		return "other", nil
	}
}

func processVideoForStart(filePath string) (string, error) {
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy",
		"-movflags", "faststart", "-f", "mp4", filePath + ".processed")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return filePath + ".processed", nil
}


type VideoM struct {
	Streams []struct {
		Index              int    `json:"index"`
		CodecName          string `json:"codec_name,omitempty"`
		CodecLongName      string `json:"codec_long_name,omitempty"`
		Profile            string `json:"profile,omitempty"`
		CodecType          string `json:"codec_type"`
		CodecTagString     string `json:"codec_tag_string"`
		CodecTag           string `json:"codec_tag"`
		Width              int    `json:"width,omitempty"`
		Height             int    `json:"height,omitempty"`
		CodedWidth         int    `json:"coded_width,omitempty"`
		CodedHeight        int    `json:"coded_height,omitempty"`
		HasBFrames         int    `json:"has_b_frames,omitempty"`
		SampleAspectRatio  string `json:"sample_aspect_ratio,omitempty"`
		DisplayAspectRatio string `json:"display_aspect_ratio,omitempty"`
		PixFmt             string `json:"pix_fmt,omitempty"`
		Level              int    `json:"level,omitempty"`
		ColorRange         string `json:"color_range,omitempty"`
		ColorSpace         string `json:"color_space,omitempty"`
		ColorTransfer      string `json:"color_transfer,omitempty"`
		ColorPrimaries     string `json:"color_primaries,omitempty"`
		ChromaLocation     string `json:"chroma_location,omitempty"`
		FieldOrder         string `json:"field_order,omitempty"`
		Refs               int    `json:"refs,omitempty"`
		IsAvc              string `json:"is_avc,omitempty"`
		NalLengthSize      string `json:"nal_length_size,omitempty"`
		ID                 string `json:"id"`
		RFrameRate         string `json:"r_frame_rate"`
		AvgFrameRate       string `json:"avg_frame_rate"`
		TimeBase           string `json:"time_base"`
		StartPts           int    `json:"start_pts"`
		StartTime          string `json:"start_time"`
		DurationTs         int    `json:"duration_ts"`
		Duration           string `json:"duration"`
		BitRate            string `json:"bit_rate,omitempty"`
		BitsPerRawSample   string `json:"bits_per_raw_sample,omitempty"`
		NbFrames           string `json:"nb_frames"`
		ExtradataSize      int    `json:"extradata_size"`
		Disposition        struct {
			Default         int `json:"default"`
			Dub             int `json:"dub"`
			Original        int `json:"original"`
			Comment         int `json:"comment"`
			Lyrics          int `json:"lyrics"`
			Karaoke         int `json:"karaoke"`
			Forced          int `json:"forced"`
			HearingImpaired int `json:"hearing_impaired"`
			VisualImpaired  int `json:"visual_impaired"`
			CleanEffects    int `json:"clean_effects"`
			AttachedPic     int `json:"attached_pic"`
			TimedThumbnails int `json:"timed_thumbnails"`
			NonDiegetic     int `json:"non_diegetic"`
			Captions        int `json:"captions"`
			Descriptions    int `json:"descriptions"`
			Metadata        int `json:"metadata"`
			Dependent       int `json:"dependent"`
			StillImage      int `json:"still_image"`
			Multilayer      int `json:"multilayer"`
		} `json:"disposition"`
		Tags struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
			VendorID    string `json:"vendor_id"`
			Encoder     string `json:"encoder"`
			Timecode    string `json:"timecode"`
		} `json:"tags"`
		SampleFmt      string `json:"sample_fmt,omitempty"`
		SampleRate     string `json:"sample_rate,omitempty"`
		Channels       int    `json:"channels,omitempty"`
		ChannelLayout  string `json:"channel_layout,omitempty"`
		BitsPerSample  int    `json:"bits_per_sample,omitempty"`
		InitialPadding int    `json:"initial_padding,omitempty"`
	} `json:"streams"`
}

