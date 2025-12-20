package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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


	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, 500, "Could not parse", err)
		return
	}
	f, h, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, 500, "could not parse file", err)
		return
	}
	defer f.Close()
	mediaType := h.Header.Get("Content-Type")
	fileExtension := strings.TrimPrefix(mediaType, "image/");

	if !strings.EqualFold("png", fileExtension) && !strings.EqualFold("jpeg", fileExtension){
		respondWithError(w, 500, "Format not supported", nil);
		return
	}
	
	
	vi, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Not owner", err)
		return
	}
	key := make([]byte, 32)
	rand.Read(key)
	filename := base64.RawURLEncoding.EncodeToString(key)

	fpath := filepath.Join(cfg.assetsRoot, filename + "." + fileExtension)
	nfile, err := os.Create(fpath)
	if err != nil {
		respondWithError(w, 500, "Could not create file", err)
		return
	}
	defer nfile.Close()
	nw, err := io.Copy(nfile, f)
	if err != nil {
		respondWithError(w, 500, "could not copy content", err)
		return
	}
	fmt.Println(nw)
	
	thumurl := "http://localhost:" + cfg.port + "/" + fpath
	vi.ThumbnailURL = &thumurl
	fmt.Println(thumurl)
	
	err = cfg.db.UpdateVideo(vi)
	if err != nil {
		respondWithError(w, 500, "Could not update video database", err)
		return
	}
	respondWithJSON(w, http.StatusOK, vi)
}
