package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type filesData struct {
	Name     string
	Location string
	ModTime  time.Time
}

type pathsConfig struct {
	InputPath  string `json:"inputPath"`
	OutputPath string `json:"outputPath"`
}

func visitFileInfos(path string, info os.FileInfo, err error) (filesData, bool) {
	if err != nil {
		fmt.Printf("Failed accessing the path %q: %v\n", path, err)
		return filesData{}, false
	}
	if info.IsDir() {
		return filesData{}, false
	}
	return filesData{
		Name:     info.Name(),
		Location: path,
		ModTime:  info.ModTime(),
	}, true
}

func gatherFiles(path string) ([]filesData, error) {
	var files []filesData
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if filepath.Base(path) == "trash" { // ignore the trash folder when walking through the directory
			return filepath.SkipDir
		}
		fileinfo, ok := visitFileInfos(path, info, err)
		if ok {
			files = append(files, fileinfo)
		}
		return nil
	})
	return files, err
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func createIfNotExist(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		errDir := os.MkdirAll(dir, 0755)
		if errDir != nil {
			log.Println("Error creating directory", errDir)
			os.Exit(1)
		}
	}
}

func moveToTrash(src, dest string) error {
	return os.Rename(src, dest)
}

func main() {
	// Create new ticker which ticks every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Launch the process for the first time
	process()

	for {
		select {
		// this case statement is run whenever the ticker ticks (every 5 minutes)
		case <-ticker.C:
			process()
		}
	}
}

func process() {
	processStart := time.Now()
	data, err := os.ReadFile("./path.json")
	if err != nil {
		log.Fatal("Failed reading path.json: ", err)
	}
	var config pathsConfig
	if err = json.Unmarshal(data, &config); err != nil {
		log.Fatal("Failed parsing path.json: ", err)
	}

	createIfNotExist(config.OutputPath)
	trashPath := filepath.Join(config.OutputPath, "trash")
	createIfNotExist(trashPath)

	filesInInputPath, err := gatherFiles(config.InputPath)
	if err != nil {
		log.Fatal("Unable to gather files from input path: ", err)
	}
	filesInOutputPath, err := gatherFiles(config.OutputPath)
	if err != nil {
		log.Fatal("Unable to gather files from output path: ", err)
	}

	inFilesMap := make(map[string]filesData)
	outFilesMap := make(map[string]filesData)
	for _, file := range filesInInputPath {
		relativePath, _ := filepath.Rel(config.InputPath, file.Location)
		inFilesMap[relativePath] = file
	}
	for _, file := range filesInOutputPath {
		relativePath, _ := filepath.Rel(config.OutputPath, file.Location)
		outFilesMap[relativePath] = file
	}

	f, err := os.OpenFile("file_error.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("Failed opening file_error.log: ", err)
	}
	defer f.Close()
	logger := log.New(f, "", log.LstdFlags)

	for _, output := range filesInOutputPath {
		relativePath, _ := filepath.Rel(config.OutputPath, output.Location)
		if _, exists := inFilesMap[relativePath]; !exists {
			fileExtension := filepath.Ext(output.Name)
			fileName := strings.TrimSuffix(output.Name, fileExtension)
			timeStamp := "." + processStart.Format("20060102150405")
			newName := fileName + timeStamp + fileExtension
			newPath := filepath.Join(trashPath, newName)
			createIfNotExist(filepath.Dir(newPath))
			err := moveToTrash(output.Location, newPath)
			if err != nil {
				logger.Println("Failed moving file to trash: ", output.Name)
			}
		}
	}

	for _, input := range filesInInputPath {
		relativePath, _ := filepath.Rel(config.InputPath, input.Location)
		source := filepath.Join(config.InputPath, relativePath)
		destination := filepath.Join(config.OutputPath, relativePath)

		if output, exists := outFilesMap[relativePath]; exists {
			// If file exists in output directory, only replace if the input file is newer
			if output.ModTime.After(input.ModTime) {
				continue
			}
		}

		createIfNotExist(filepath.Dir(destination))
		err := copyFile(source, destination)
		if err != nil {
			logger.Println("Failed copying file: ", input.Name)
		}
	}
}
