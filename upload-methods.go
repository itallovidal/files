package files

import (
	"mime"
	"os"
	"strconv"
	"strings"

	"github.com/go-bolo/bolo"
	files_database "github.com/go-bolo/files/database"
	files_helpers "github.com/go-bolo/files/helpers"
	files_processor "github.com/go-bolo/files/processor"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var defaultExtension = "webp"
var defaultMime = "image/webp"

func UploadFileFromLocalhost(fileName string, description string, filePath string, storageName string, record *FileModel, app bolo.App) error {
	var err error
	filePlugin := app.GetPlugin("files").(*FilePlugin)
	storage := filePlugin.GetStorage(storageName)

	fileUUID := uuid.New().String()

	mimeType, extension, _ := files_helpers.GetFileExtensionAndMimeType(filePath)
	if extension != "" {
		record.Extension = &extension
		record.Mime = &mimeType
	} else {
		fileNameSplits := strings.Split(fileName, ".")
		if len(fileNameSplits) > 1 {
			ext := fileNameSplits[len(fileNameSplits)-1]
			record.Extension = &ext
			fileUUID += "." + ext
		}
	}

	fileStatus, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	size := fileStatus.Size()

	record.Active = true
	record.Name = fileUUID
	record.Description = &description
	record.Size = &size
	record.Originalname = fileName
	record.StorageName = storageName

	originalDest, _ := storage.GetUploadPathFromFile("original", "", record)

	err = storage.UploadFile(record, filePath, originalDest)
	if err != nil {
		return errors.Wrap(err, "UploadFileFromLocalhost Error on upload file")
	}

	urls := files_database.ImageURLsField{}
	urls["original"], _ = storage.GetUrlFromFile("original", record)

	record.SetURLs(urls)

	return nil
}

func UploadImageFromLocalhost(fileName string, description string, filePath string, storageName string, record *ImageModel, app bolo.App) error {
	var err error
	filePlugin := app.GetPlugin("files").(*FilePlugin)
	storage := filePlugin.GetStorage(storageName)
	processor := filePlugin.Processor
	fileUUID := uuid.New().String()
	styles := filePlugin.ImageStyles

	_, originalExtension, _ := files_helpers.GetFileExtensionAndMimeType(filePath)
	if originalExtension == "" {
		// Fallback to filename extension if detection fails
		fileNameSplits := strings.Split(fileName, ".")
		if len(fileNameSplits) > 1 {
			originalExtension = fileNameSplits[len(fileNameSplits)-1]
		}
	}

	// Check if original format should be ignored
	shouldIgnoreFormat := false
	if filePlugin.ImageFormatToIgnore != "" && originalExtension != "" {
		ignoreFormats := strings.Split(filePlugin.ImageFormatToIgnore, ",")
		for _, ignoreFormat := range ignoreFormats {
			ignoreFormat = strings.TrimSpace(ignoreFormat)
			if strings.EqualFold(originalExtension, ignoreFormat) {
				shouldIgnoreFormat = true
				break
			}
		}
	}

	if filePlugin.ImageFormat != "" && !shouldIgnoreFormat {
		defaultExtension = filePlugin.ImageFormat
		defaultMime = mime.TypeByExtension("." + filePlugin.ImageFormat)
	}

	fileStatus, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	size := fileStatus.Size()

	record.Active = true
	record.Description = &description
	record.Size = &size
	record.Originalname = fileName
	record.StorageName = storageName

	if shouldIgnoreFormat && originalExtension != "" {
		record.Extension = &originalExtension
		originalMime := mime.TypeByExtension("." + originalExtension)
		if originalMime == "" {
			originalMime = "application/octet-stream" // fallback
		}
		record.Mime = &originalMime
	} else {
		record.Extension = &defaultExtension
		record.Mime = &defaultMime
	}

	var resizeOpts files_processor.Options

	// ORIGINAL:
	if style, ok := styles["original"]; ok {
		// set original format to override / resize default format
		resizeOpts = files_processor.Options{
			"width":  strconv.Itoa(int(style.Width)),
			"height": strconv.Itoa(int(style.Height)),
		}
	} else {
		// Default:
		resizeOpts = files_processor.Options{
			"width":  strconv.Itoa(int(filePlugin.MaxImageWidth)),
			"height": strconv.Itoa(int(filePlugin.MaxImageHeight)),
		}
	}

	if filePlugin.ImageFormat != "" && !shouldIgnoreFormat {
		record.Name = fileUUID + "." + filePlugin.ImageFormat
		resizeOpts["format"] = filePlugin.ImageFormat
	} else if shouldIgnoreFormat && originalExtension != "" {
		record.Name = fileUUID + "." + originalExtension
	} else {
		record.Name = fileUUID
	}

	if resizeOpts["format"] == "" {
		if shouldIgnoreFormat && originalExtension != "" {
			resizeOpts["format"] = originalExtension
		} else {
			resizeOpts["format"] = defaultExtension
		}
	}

	// Determine the format to use for storage path
	storageFormat := ""
	if shouldIgnoreFormat && originalExtension != "" {
		storageFormat = originalExtension
	} else if filePlugin.ImageFormat != "" {
		storageFormat = filePlugin.ImageFormat
	}

	originalDest, _ := storage.GetUploadPathFromFile("original", storageFormat, record)

	// Skip resize processing for ignored formats to preserve their properties (e.g., GIF animation, SVG vectors)
	if shouldIgnoreFormat && originalExtension != "" {
		// For ignored formats, just copy the file without processing
		err = storage.UploadFile(record, filePath, originalDest)
		if err != nil {
			return errors.Wrap(err, "UploadImageFromLocalhost Error on upload file")
		}
	} else {
		// Process the image with resize/format conversion
		err = processor.Resize(filePath, filePath, record.Name, resizeOpts)
		if err != nil {
			return err
		}

		err = storage.UploadFile(record, filePath, originalDest)
		if err != nil {
			return errors.Wrap(err, "UploadImageFromLocalhost Error on upload file")
		}
	}

	record.ResetURLs(app)

	return nil
}
