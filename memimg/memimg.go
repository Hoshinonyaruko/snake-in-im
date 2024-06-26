package memimg

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

var (
	Avatars      map[string]image.Image
	AvatarsMutex sync.RWMutex
	foods        map[string]image.Image
	foodsMutex   sync.RWMutex
)

func LoadAvatars(directory string) error {
	Avatars = make(map[string]image.Image)
	return filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			img, err := LoadImage(path)
			if err != nil {
				return err
			}
			Avatars[filepath.Base(path)] = img
		}
		return nil
	})
}

func LoadImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func GetAvatarFromMemory(filename string) (image.Image, bool) {
	AvatarsMutex.RLock()
	img, exists := Avatars[filename]
	AvatarsMutex.RUnlock()
	return img, exists
}

func LoadFoods(directory string) error {
	foods = make(map[string]image.Image)
	return filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			img, err := LoadImage(path)
			if err != nil {
				return err
			}
			foods[filepath.Base(path)] = img
		}
		return nil
	})
}

func WatchFoods(directory string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					img, err := LoadImage(event.Name)
					if err == nil {
						foodsMutex.Lock()
						foods[filepath.Base(event.Name)] = img
						foodsMutex.Unlock()
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(directory)
	if err != nil {
		panic(err)
	}
	<-done
}

func GetFoodFromMemory(filename string) (image.Image, bool) {
	foodsMutex.RLock()
	img, exists := foods[filename]
	foodsMutex.RUnlock()
	return img, exists
}
