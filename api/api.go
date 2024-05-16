package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fogleman/gg"
	"github.com/gin-gonic/gin"
	"github.com/hoshinonyaruko/snake-in-im/config"
	"github.com/hoshinonyaruko/snake-in-im/memimg"
	"github.com/hoshinonyaruko/snake-in-im/snake"
	"github.com/hoshinonyaruko/snake-in-im/sqlite"
	"github.com/hoshinonyaruko/snake-in-im/structs"
	_ "github.com/mattn/go-sqlite3"
)

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
			err := snake.ProcessAndSaveAvatar(avatarUrl, openID)
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
		renderImageAndSave(&gameMap.Map, groupID, openID) // Render the map and save as an image

		imageUrl := fmt.Sprintf("http://%s/static/%s.jpg", config.GetConfigValue("selfpath"), groupID)
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
		game.Map.Food = []structs.Position{snake.GenerateRandomPositionWithAvatar(game.Map.Width, game.Map.Height, "food.png")}

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
	// 固定每个方块为20像素
	const blockSize = 20
	canvasWidth := gameMap.Width * blockSize
	canvasHeight := gameMap.Height * blockSize

	// 创建一个新的绘图上下文
	dc := gg.NewContext(canvasWidth, canvasHeight)

	// 从内存中获取背景图片
	backgroundFileName := fmt.Sprintf("%s_blur.jpg", openID)
	bgImg, found := memimg.GetAvatarFromMemory(backgroundFileName)
	if found {
		// 计算缩放比例
		bgWidth := float64(bgImg.Bounds().Dx())
		bgHeight := float64(bgImg.Bounds().Dy())
		scaleX := float64(canvasWidth) / bgWidth
		scaleY := float64(canvasHeight) / bgHeight

		// 使用更大的缩放比例以确保背景完全覆盖画布
		scale := math.Max(scaleX, scaleY)

		// 设置图像缩放
		dc.Scale(scale, scale)

		// 由于缩放后，图像的原点也会相应改变，需要重新定位图像
		offsetX := (float64(canvasWidth) - bgWidth*scale) / 2.0 / scale
		offsetY := (float64(canvasHeight) - bgHeight*scale) / 2.0 / scale

		dc.DrawImage(bgImg, int(offsetX), int(offsetY))
	} else {
		dc.SetRGB(1, 1, 1) // 如果背景图未找到，使用白色背景
		dc.Clear()
	}

	// 还原缩放和偏移设置以便绘制其他元素
	dc.Identity()

	// 绘制网格
	dc.SetRGB(0.9, 0.9, 0.9) // 设置网格颜色为浅灰
	for x := 0; x <= canvasWidth; x += blockSize {
		dc.DrawLine(float64(x), 0, float64(x), float64(canvasHeight))
		dc.Stroke()
	}

	for y := 0; y <= canvasHeight; y += blockSize {
		dc.DrawLine(0, float64(y), float64(canvasWidth), float64(y))
		dc.Stroke()
	}

	// 绘制每条蛇
	for _, snake := range gameMap.Snakes {
		for _, pos := range snake.Positions {
			img, found := memimg.GetAvatarFromMemory(pos.Avatar)
			if found {
				// 缩放图片以匹配 blockSize x blockSize 的尺寸
				scaledImg := scaleImage(img, blockSize, blockSize)
				// 在正确的位置绘制缩放后的图片
				dc.DrawImage(scaledImg, pos.X*blockSize, pos.Y*blockSize)
			} else {
				dc.SetRGB(0, 0, 0) // 如果图片加载失败，使用黑色表示该位置
				dc.DrawRectangle(float64(pos.X*blockSize), float64(pos.Y*blockSize), float64(blockSize), float64(blockSize))
				dc.Fill()
			}
		}
	}

	// 绘制食物
	for _, foodPos := range gameMap.Food {
		// 从内存中获取对应的食物图像
		foodImg, found := memimg.GetAvatarFromMemory(foodPos.Avatar)
		if found {
			// 计算食物在画布上的坐标并绘制
			dc.DrawImageAnchored(foodImg, foodPos.X*blockSize, foodPos.Y*blockSize, 0, 0)
		} else {
			// 如果食物图片未找到，使用黑色矩形表示该食物位置
			dc.SetRGB(0, 0, 0)
			dc.DrawRectangle(float64(foodPos.X*blockSize), float64(foodPos.Y*blockSize), float64(blockSize), float64(blockSize))
			dc.Fill()
		}
	}

	// 保存图片
	fileName := fmt.Sprintf("./output/%s.png", groupID)
	os.MkdirAll(filepath.Dir(fileName), os.ModePerm)
	return dc.SavePNG(fileName)
}

// scaleImage 重新设计以保持图片质量
func scaleImage(img image.Image, newWidth, newHeight int) image.Image {
	// 创建一个与目标尺寸相同的画布
	dc := gg.NewContext(newWidth, newHeight)

	// 计算缩放比例
	sx := float64(newWidth) / float64(img.Bounds().Dx())
	sy := float64(newHeight) / float64(img.Bounds().Dy())

	// 绘制缩放后的图片
	dc.Scale(sx, sy)
	dc.DrawImage(img, 0, 0)

	// 返回缩放后的图片
	return dc.Image()
}
