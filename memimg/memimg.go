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
	avatars      map[string]image.Image
	avatarsMutex sync.RWMutex
)

func LoadAvatars(directory string) error {
	avatars = make(map[string]image.Image)
	return filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			img, err := loadImage(path)
			if err != nil {
				return err
			}
			avatars[filepath.Base(path)] = img
		}
		return nil
	})
}

func loadImage(path string) (image.Image, error) {
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

func WatchAvatars(directory string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err) // 实际开发中应该更优雅地处理错误
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
					img, err := loadImage(event.Name)
					if err == nil {
						avatarsMutex.Lock()
						avatars[filepath.Base(event.Name)] = img
						avatarsMutex.Unlock()
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
		panic(err) // 实际开发中应该更优雅地处理错误
	}
	<-done
}

func GetAvatarFromMemory(filename string) (image.Image, bool) {
	avatarsMutex.RLock()
	img, exists := avatars[filename]
	avatarsMutex.RUnlock()
	return img, exists
}
