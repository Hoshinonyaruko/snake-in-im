package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
	"github.com/gin-gonic/gin"
	"github.com/hoshinonyaruko/snake-in-im/config"
	"github.com/hoshinonyaruko/snake-in-im/memimg"
	"github.com/hoshinonyaruko/snake-in-im/snake"
	"github.com/hoshinonyaruko/snake-in-im/sqlite"
	"github.com/hoshinonyaruko/snake-in-im/structs"
	_ "github.com/mattn/go-sqlite3"
)

// 全局缓存
var drawingCache sync.Map

func InitDB() *sql.DB {
	db, err := sql.Open("sqlite3", "game.db")
	if err != nil {
		log.Fatal(err)
	}

	sqlite.InitializeDatabase(db)

	return db
}

func UpdateDirection(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		groupID := c.Query("groupid")
		openID := c.Query("openid")
		newDirection := c.Query("direction")

		// 验证是否提供了必要的查询参数
		if groupID == "" || openID == "" || newDirection == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required query parameters: groupid, openid, or direction"})
			return
		}

		// 更新蛇的方向
		if err := updateSnakeDirection(db, groupID, openID, newDirection); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update direction"})
			return
		}

		// 返回成功响应
		c.JSON(http.StatusOK, gin.H{"message": "Direction updated successfully"})
	}
}

func updateSnakeDirection(db *sql.DB, groupID, openID, newDirection string) error {
	// 定义合法的方向集合
	validDirections := map[string]bool{
		"up":    true,
		"down":  true,
		"left":  true,
		"right": true,
	}

	// 检查新方向是否合法
	if _, valid := validDirections[newDirection]; !valid {
		return fmt.Errorf("invalid direction '%s' provided", newDirection)
	}

	// 执行更新操作
	result, err := db.Exec("UPDATE Snakes SET Direction = ? WHERE GroupID = ? AND OpenID = ?", newDirection, groupID, openID)
	if err != nil {
		return err
	}

	// 确认是否确实更新了某条记录
	if count, err := result.RowsAffected(); err != nil || count == 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("no snake found with the specified groupID and openID, or the direction is already '%s'", newDirection)
	}

	return nil
}

func RenderMapHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		groupID := c.Query("groupid")
		openID := c.Query("openid")
		avatarUrl, _ := url.QueryUnescape(c.Query("avatarUrl")) // Decode the URL
		width, _ := strconv.Atoi(c.DefaultQuery("width", "20"))
		height, _ := strconv.Atoi(c.DefaultQuery("height", "20"))
		refreshInterval, _ := strconv.Atoi(c.DefaultQuery("refresh_interval", "0"))
		foodName := c.Query("foodname")

		if avatarUrl != "" {
			// Process and save the avatar
			err := snake.ProcessAndSaveAvatar(avatarUrl, openID, config.GetConfigValue("blocksize").(int))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to process avatar"})
				return
			}
		}

		// 获取&创建当前群游戏地图
		gameMap, err := getOrCreateGameMap(db, groupID, width, height, refreshInterval)
		if err != nil {
			fmt.Printf("err getOrCreateGameMap :%v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to fetch or create game map"})
			return
		}

		// 贪食蛇刷新
		snake.UpdateGameMapIfNeeded(gameMap, openID) // Check and update the game map state
		// 食物刷新
		if foodName != "" {
			snake.AddFoodToGameMap(gameMap, foodName)
		}
		// 绘图
		renderImageAndSave(&gameMap.Map, groupID, openID) // Render the map and save as an image

		imageUrl := fmt.Sprintf("http://%s/static/%s.jpg", config.GetConfigValue("selfpath").(string), groupID)
		c.JSON(http.StatusOK, gin.H{"image_url": imageUrl})

		// 持久化
		sqlite.UpdateGameMapInDB(db, gameMap)
	}
}

func getOrCreateGameMap(db *sql.DB, groupID string, width, height, refreshInterval int) (*structs.Game, error) {
	var game structs.Game

	// Check and try to get the existing game map
	var lastRefresh time.Time
	err := db.QueryRow("SELECT GroupID, MapWidth, MapHeight, LastRefresh, RefreshInterval FROM Games WHERE GroupID = ?", groupID).Scan(
		&game.GroupID, &game.Map.Width, &game.Map.Height, &lastRefresh, &game.RefreshInterval,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	game.LastRefresh = lastRefresh.Unix() // 转换为 Unix 时间戳

	if err == sql.ErrNoRows {
		// Game map does not exist, create a new one
		if refreshInterval == 0 {
			refreshInterval = 3600 // Default refresh interval to one hour if not specified
		}
		game.RefreshInterval = refreshInterval
		game.GroupID = groupID
		game.Map.Width = width
		game.Map.Height = height
		game.LastRefresh = time.Now().Unix()

		// Insert a new game record
		_, err = db.Exec("INSERT INTO Games (GroupID, MapWidth, MapHeight, LastRefresh, RefreshInterval) VALUES (?, ?, ?, ?, ?)",
			groupID, width, height, game.LastRefresh, refreshInterval)
		if err != nil {
			return nil, err
		}

		// Initialize empty snakes map and food position
		game.Map.Snakes = make(map[string]structs.Snake)
		// 初始化食物位置
		game.Map.Food = []structs.Position{snake.GenerateRandomPositionWithAvatar(game.Map.Width, game.Map.Height, "food_small.png")}

	} else {
		// Load snakes
		rows, err := db.Query("SELECT OpenID, Positions, Direction FROM Snakes WHERE GroupID = ?", groupID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		game.Map.Snakes = make(map[string]structs.Snake)
		var posData string
		for rows.Next() {
			var snake structs.Snake
			// 注意，我们不再从数据库读取Avatar，因为每个Position已经包含Avatar
			if err := rows.Scan(&snake.OpenID, &posData, &snake.Direction); err != nil {
				return nil, err
			}
			// 反序列化Position数据，其中每个Position包含了Avatar信息
			if err := json.Unmarshal([]byte(posData), &snake.Positions); err != nil {
				return nil, err
			}
			game.Map.Snakes[snake.OpenID] = snake
		}

		// Load food position
		var foodPositions []structs.Position
		rows, err = db.Query("SELECT Position FROM Foods WHERE GroupID = ?", game.GroupID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var posData string
			if err := rows.Scan(&posData); err != nil {
				return nil, err
			}
			var pos structs.Position
			if err := json.Unmarshal([]byte(posData), &pos); err != nil {
				return nil, err
			}
			foodPositions = append(foodPositions, pos)
		}
		game.Map.Food = foodPositions

		// Update the refresh interval if provided
		if refreshInterval != 0 && refreshInterval != game.RefreshInterval {
			_, err = db.Exec("UPDATE Games SET RefreshInterval = ? WHERE GroupID = ?", refreshInterval, groupID)
			if err != nil {
				return nil, err
			}
			game.RefreshInterval = refreshInterval
		}
	}

	return &game, nil
}

// renderImageAndSave 渲染地图并保存为图片
func renderImageAndSave(gameMap *structs.GameMap, groupID, openID string) error {
	var dc *gg.Context
	// 从配置中读取
	blockSize := config.GetConfigValue("blocksize").(int)
	canvasWidth := gameMap.Width * blockSize
	canvasHeight := gameMap.Height * blockSize

	// 构造缓存键
	cacheKey := fmt.Sprintf("%s_%s_%d", groupID, openID, blockSize)

	// 尝试从缓存中获取已经绘制好的图像
	if cachedImg, ok := drawingCache.Load(cacheKey); ok {
		dc = cachedImg.(*gg.Context)
	} else {
		// 如果缓存未命中，创建一个新的绘图上下文
		dc = gg.NewContext(canvasWidth, canvasHeight)
		// 加载并缩放背景图片
		renderAndCacheBackground(dc, openID, canvasWidth, canvasHeight)

		// 绘制网格等其他元素
		renderGrid(dc, canvasWidth, canvasHeight, blockSize)
		// 将完成的绘图上下文保存到缓存中
		drawingCache.Store(cacheKey, dc)
	}

	width := gameMap.Width * blockSize
	height := gameMap.Height * blockSize

	// 创建总的画布
	finalDC := gg.NewContext(width, height)
	finalDC.DrawImage(dc.Image(), 0, 0)

	// 创建等待组来同步所有goroutines
	var wg sync.WaitGroup

	// 分片处理每个部分的绘图
	for _, snake := range gameMap.Snakes {
		wg.Add(1)
		go func(snake structs.Snake) {
			defer wg.Done()
			// 创建独立的绘图上下文
			dc := gg.NewContext(width, height)
			for _, pos := range snake.Positions {
				img, found := memimg.GetAvatarFromMemory(pos.Avatar)
				if found {
					dc.DrawImage(img, pos.X*blockSize, pos.Y*blockSize)
				} else {
					// 从内存食物中获取对应的食物图像
					foodImg, found := memimg.GetFoodFromMemory(pos.Avatar)
					if found {
						dc.DrawImage(foodImg, pos.X*blockSize, pos.Y*blockSize)
					} else {
						// 如果食物图片未找到，使用黑色矩形表示该食物位置
						dc.SetRGB(0, 0, 0) // 如果图片加载失败，使用黑色表示该位置
						dc.DrawRectangle(float64(pos.X*blockSize), float64(pos.Y*blockSize), float64(blockSize), float64(blockSize))
						dc.Fill()
					}
				}
			}
			// 画布上锁并合并到总画布
			finalDC.DrawImage(dc.Image(), 0, 0)
		}(snake)
	}

	// 食物的绘制也并行处理
	for _, foodPos := range gameMap.Food {
		wg.Add(1)
		go func(foodPos structs.Position) {
			defer wg.Done()
			dc := gg.NewContext(width, height)
			foodImg, found := memimg.GetFoodFromMemory(foodPos.Avatar)
			if found {
				dc.DrawImage(foodImg, foodPos.X*blockSize, foodPos.Y*blockSize)
			} else {
				dc.SetRGB(0, 0, 0)
				dc.DrawRectangle(float64(foodPos.X*blockSize), float64(foodPos.Y*blockSize), float64(blockSize), float64(blockSize))
				dc.Fill()
			}
			// 画布上锁并合并到总画布
			finalDC.DrawImage(dc.Image(), 0, 0)
		}(foodPos)
	}

	// 等待所有goroutines完成
	wg.Wait()

	// 保存图片
	fileName := fmt.Sprintf("./output/%s.png", groupID)
	os.MkdirAll(filepath.Dir(fileName), os.ModePerm)
	return finalDC.SavePNG(fileName)
}

func scaleImage(img image.Image, newWidth, newHeight int) image.Image {
	dc := gg.NewContext(newWidth, newHeight)
	sx := float64(newWidth) / float64(img.Bounds().Dx())
	sy := float64(newHeight) / float64(img.Bounds().Dy())
	scale := math.Min(sx, sy)
	offsetX := (float64(newWidth) - float64(img.Bounds().Dx())*scale) / 2
	offsetY := (float64(newHeight) - float64(img.Bounds().Dy())*scale) / 2
	dc.Scale(scale, scale)
	dc.DrawImage(img, int(offsetX/scale), int(offsetY/scale))
	return dc.Image()
}

func PreloadAndScaleFoods(foodDirectory string, blockSize int) {
	filepath.WalkDir(foodDirectory, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".png" {
			avatar := filepath.Base(path)
			if strings.Contains(avatar, "_small") || strings.Contains(avatar, "_blur") {
				// Skip processing if it's already a modified file
				return nil
			}

			scaledAvatar := avatar[:len(avatar)-len(filepath.Ext(avatar))] + "_small.png"
			blurAvatar := avatar[:len(avatar)-len(filepath.Ext(avatar))] + "_blur.png"

			scaledAvatarPath := filepath.Join(foodDirectory, scaledAvatar)
			blurAvatarPath := filepath.Join(foodDirectory, blurAvatar)

			// Check if scaled and blurred images already exist
			if _, err := os.Stat(scaledAvatarPath); os.IsNotExist(err) {
				img, err := memimg.LoadImage(path)
				if err != nil {
					return err
				}
				scaledImg := scaleImage(img, blockSize, blockSize)
				scaledOutFile, err := os.Create(scaledAvatarPath)
				if err != nil {
					return err
				}
				defer scaledOutFile.Close()
				png.Encode(scaledOutFile, scaledImg)
			}

			if _, err := os.Stat(blurAvatarPath); os.IsNotExist(err) {
				scaledImg, err := memimg.LoadImage(scaledAvatarPath) // Assuming memimg.LoadImage loads PNGs
				if err != nil {
					return err
				}
				blurImg := imaging.Blur(scaledImg, 3.5)
				blurOutFile, err := os.Create(blurAvatarPath)
				if err != nil {
					return err
				}
				defer blurOutFile.Close()
				png.Encode(blurOutFile, blurImg)
			}
		}
		return nil
	})
}

func renderAndCacheBackground(dc *gg.Context, openID string, width, height int) {
	backgroundFileName := fmt.Sprintf("%s_blur.jpg", openID)
	bgImg, found := memimg.GetAvatarFromMemory(backgroundFileName)
	if found {
		// 缩放并定位背景图像
		bgWidth := float64(bgImg.Bounds().Dx())
		bgHeight := float64(bgImg.Bounds().Dy())
		scale := math.Max(float64(width)/bgWidth, float64(height)/bgHeight)
		dc.Scale(scale, scale)
		offsetX := (float64(width) - bgWidth*scale) / 2.0 / scale
		offsetY := (float64(height) - bgHeight*scale) / 2.0 / scale
		dc.DrawImage(bgImg, int(offsetX), int(offsetY))
	} else {
		dc.SetRGB(1, 1, 1)
		dc.Clear()
	}
	dc.Identity()
}

func renderGrid(dc *gg.Context, width, height, blockSize int) {
	dc.SetRGB(0.9, 0.9, 0.9)
	for x := 0; x <= width; x += blockSize {
		dc.DrawLine(float64(x), 0, float64(x), float64(height))
		dc.Stroke()
	}
	for y := 0; y <= height; y += blockSize {
		dc.DrawLine(0, float64(y), float64(width), float64(y))
		dc.Stroke()
	}
}
