package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

/*
Complete the (currently empty) handlerUploadVideo handler to store video files in S3. Images will stay on the local file system for now. I recommend using the image upload handler as a reference.
- Set an upload limit of 1 GB (1 << 30 bytes) using http.MaxBytesReader.
- Extract the videoID from the URL path parameters and parse it as a UUID
- Authenticate the user to get a userID
- Get the video metadata from the database, if the user is not the video owner, return a http.StatusUnauthorized response
- Parse the uploaded video file from the form data
  - Use (http.Request).FormFile with the key "video" to get a multipart.File in memory
  - Remember to defer closing the file with (os.File).Close - we don't want any memory leaks

- Validate the uploaded file to ensure it's an MP4 video
  - Use mime.ParseMediaType and "video/mp4" as the MIME type

- Save the uploaded file to a temporary file on disk.
  - Use os.CreateTemp to create a temporary file. I passed in an empty string for the directory to use the system default, and the name "tubely-upload.mp4" (but you can use whatever you want)
  - defer remove the temp file with os.Remove
  - defer close the temp file (defer is LIFO, so it will close before the remove)
  - io.Copy the contents over from the wire to the temp file

- Reset the tempFile's file pointer to the beginning with .Seek(0, io.SeekStart) - this will allow us to read the file again from the beginning
- Put the object into S3 using PutObject. You'll need to provide:
  - The bucket name
  - The file key. Use the same <random-32-byte-hex>.ext format as the key. e.g. 1a2b3c4d5e6f7890abcd1234ef567890.mp4
  - The file contents (body). The temp file is an os.File which implements io.Reader
  - Content type, which is the MIME type of the file.

- Update the VideoURL of the video record in the database with the S3 bucket and key. S3 URLs are in the format https://<bucket-name>.s3.<region>.amazonaws.com/<key>. Make sure you use the correct region and bucket name!
- Restart your server and test the handler by uploading the boots-video-vertical.mp4 file. Make sure that:
  - The video is correctly uploaded to your S3 bucket.
  - The video_url in your database is updated with the S3 bucket and key (and thus shows up in the web UI)
*/
func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// Set 1GB upload limit
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	// Extract and validate video ID
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video ID", err)
		return
	}

	// Authenticate user
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

	// Get video metadata and check ownership
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get video", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You don't own this video", nil)
		return
	}

	// Parse the multipart form to get the file
	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error getting video from form", err)
		return
	}
	defer file.Close()

	// Validate file type
	mediaType, _, err := mime.ParseMediaType(fileHeader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type header", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "File type not allowed. Only MP4 videos are supported.", nil)
		return
	}

	// Create temporary file
	tempFile, err := os.CreateTemp("", "tubely-upload-*.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temporary file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy uploaded file to temporary file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save file", err)
		return
	}

	// Reset file pointer to beginning
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't reset file pointer", err)
		return
	}

	// Generate random filename for S3
	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random filename", err)
		return
	}
	filename := hex.EncodeToString(randomBytes) + ".mp4"

	// Upload to S3
	_, err = cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &filename,
		Body:        tempFile,
		ContentType: &mediaType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload file to S3", err)
		return
	}

	// Create S3 URL
	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, filename)
	video.VideoURL = &videoURL

	// Update video metadata in database
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video metadata", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
