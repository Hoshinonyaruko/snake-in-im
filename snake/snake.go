// 关于的蛇的更新
package snake

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/disintegration/imaging"
	"github.com/hoshinonyaruko/snake-in-im/structs"
)

// 处理和保存头像到avatar文件夹
func ProcessAndSaveAvatar(avatarUrl, openID string) error {
	// Download the avatar image
	resp, err := http.Get(avatarUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the image data from response
	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Decode the image data
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return err
	}

	// Write the original image data to a file
	avatarPath := fmt.Sprintf("./avatar/%s.jpg", openID)
	err = os.WriteFile(avatarPath, imgData, 0644)
	if err != nil {
		return err
	}

	// Apply Gaussian Blur using the imaging library
	blurredImg := imaging.Blur(img, 15) // Adjust the sigma parameter to control the strength of the blur

	// Save the blurred image
	blurredAvatarPath := fmt.Sprintf("./avatar/%s_blur.jpg", openID)
	outFile, err := os.Create(blurredAvatarPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	jpeg.Encode(outFile, blurredImg, &jpeg.Options{Quality: 95}) // Save with high quality

	return nil
}

func UpdateGameMapIfNeeded(game *structs.Game, openID string) error {
	currentTime := time.Now().Unix()
	elapsed := currentTime - game.LastRefresh
	fmt.Printf("elapsed[%v] game.LastRefresh[%v]\n", elapsed, game.LastRefresh)

	// 计算应该执行的移动次数
	moveInterval := int64(game.RefreshInterval) // 移动间隔，以秒为单位
	moveCount := elapsed / moveInterval
	if moveCount == 0 {
		fmt.Printf("没有到刷新时间,openID[%v]\n", openID)
		return nil // 还没到刷新时间
	} else {
		fmt.Printf("移动次数[%v]\n", moveCount)
	}

	// 检查并添加新蛇
	if _, exists := game.Map.Snakes[openID]; !exists {
		newPos := GenerateRandomPositionWithAvatar(game.Map.Width, game.Map.Height, fmt.Sprintf("%s.jpg", openID))

		// 随机选择一个方向
		directions := []string{"up", "down", "left", "right"}
		randomDirection := directions[rand.Intn(len(directions))] // 随机选择一个索引

		// 创建并添加新蛇
		game.Map.Snakes[openID] = structs.Snake{
			Positions: []structs.Position{newPos},
			OpenID:    openID,
			Direction: randomDirection, // 使用随机方向
		}
	}

	// 循环执行移动和碰撞检测
	for i := int64(0); i < moveCount; i++ {
		for id, snake := range game.Map.Snakes {
			game.Map.Snakes[id] = MoveSnake(snake, game.Map.Width, game.Map.Height)
		}
		CheckCollisions(&game.Map)
	}

	// 刷新新的时间
	game.LastRefresh = currentTime

	return nil
}

// 辅助函数：生成带有头像的随机位置 一条新的蛇
func GenerateRandomPositionWithAvatar(width, height int, avatar string) structs.Position {
	return structs.Position{
		X:      rand.Intn(width),
		Y:      rand.Intn(height),
		Avatar: avatar, // 为新位置设置头像
	}
}

func MoveSnake(snake structs.Snake, width, height int) structs.Snake {
	head := snake.Positions[0]
	var newX, newY int

	// 根据方向计算新头部位置
	switch snake.Direction {
	case "up":
		newX, newY = head.X, head.Y-1
	case "down":
		newX, newY = head.X, head.Y+1
	case "left":
		newX, newY = head.X-1, head.Y
	case "right":
		newX, newY = head.X+1, head.Y
	}

	// 处理新位置可能超出边界的情况
	newX, newY = WrapPosition(newX, newY, width, height)

	// 创建新的头部位置，保留原头部的avatar属性
	newHead := structs.Position{X: newX, Y: newY, Avatar: head.Avatar}

	// 将新头部添加到位置列表，并且移除尾部，以保持蛇的长度
	newPositions := append([]structs.Position{newHead}, snake.Positions...)
	if len(newPositions) > 1 {
		newPositions = newPositions[:len(newPositions)-1] // 保持蛇身长度不变，移除最后一个元素（原尾部）
	}

	// 更新蛇的位置列表
	snake.Positions = newPositions
	return snake
}

func WrapPosition(x, y, width, height int) (int, int) {
	if x < 0 {
		x += width
	} else if x >= width {
		x -= width
	}
	if y < 0 {
		y += height
	} else if y >= height {
		y -= height
	}
	return x, y
}

func CheckCollisions(gameMap *structs.GameMap) {
	toDelete := make(map[string]bool)
	positionMap := make(map[structs.Position]string)

	// 记录每个位置的蛇ID
	for id, snake := range gameMap.Snakes {
		for _, pos := range snake.Positions {
			positionMap[pos] = id
		}
	}

	// 检查头部与其他蛇的身体部分是否重叠
	for id, snake := range gameMap.Snakes {
		head := snake.Positions[0]
		if otherID, exists := positionMap[head]; exists && otherID != id {
			ResolveCollision(gameMap.Snakes, id, otherID, toDelete)
		}
	}

	// 删除被吃掉的蛇
	for id := range toDelete {
		delete(gameMap.Snakes, id)
	}
}

func ResolveCollision(snakes map[string]structs.Snake, snake1ID, snake2ID string, toDelete map[string]bool) {
	snake1 := snakes[snake1ID]
	snake2 := snakes[snake2ID]

	if len(snake1.Positions) >= len(snake2.Positions) {
		EatSnake(snakes, snake1ID, snake2ID, toDelete)
	} else {
		EatSnake(snakes, snake2ID, snake1ID, toDelete)
	}
}

func EatSnake(snakes map[string]structs.Snake, eaterID, eatenID string, toDelete map[string]bool) {
	eater := snakes[eaterID]
	eaten := snakes[eatenID]

	eater.Positions = append(eater.Positions, eaten.Positions...) // 吞并被吃蛇的所有部分
	toDelete[eatenID] = true                                      // 标记被吃蛇为删除

	UpdateAvatar(&eater, eaten.OpenID) // 更新avatar为吃掉蛇的样式
	snakes[eaterID] = eater            // 保存回map中
}

func UpdateAvatar(snake *structs.Snake, eatenOpenID string) {
	// 更新最近吞并的蛇块的头像，将它们的头像设为被吃蛇的模糊头像
	for i := range snake.Positions {
		snake.Positions[i].Avatar = fmt.Sprintf("%s_blur.jpg", eatenOpenID)
	}
}

// AddFoodToGameMap adds a new food item to the game map
func AddFoodToGameMap(gameMap *structs.Game, foodName string) {
	newFood := structs.Position{
		X:      0,
		Y:      0,
		Avatar: foodName + ".png",
	}

	// Find a unique position that does not overlap with snakes or other food
	for {
		newFood.X = rand.Intn(gameMap.Map.Width)
		newFood.Y = rand.Intn(gameMap.Map.Height)
		if !positionOverlap(gameMap, newFood) {
			break
		}
	}

	// Add the new food position to the map
	gameMap.Map.Food = append(gameMap.Map.Food, newFood)
}

// Check if the proposed new position overlaps with any snakes or existing food
func positionOverlap(gameMap *structs.Game, pos structs.Position) bool {
	// Check overlap with food
	for _, food := range gameMap.Map.Food {
		if food.X == pos.X && food.Y == pos.Y {
			return true
		}
	}

	// Check overlap with all snakes
	for _, snake := range gameMap.Map.Snakes {
		for _, snakePos := range snake.Positions {
			if snakePos.X == pos.X && snakePos.Y == pos.Y {
				return true
			}
		}
	}

	return false
}
