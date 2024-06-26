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
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/hoshinonyaruko/snake-in-im/memimg"
	"github.com/hoshinonyaruko/snake-in-im/structs"
)

var (
	toDelete            = make(map[string]bool) // 包级变量，记录待删除的蛇
	collisionCheckCount = 0                     // 用于记录碰撞检查的次数
)

// 处理和保存头像到avatar文件夹
func ProcessAndSaveAvatar(avatarUrl, openID string, blockSize int) error {
	// 下载头像图片
	resp, err := http.Get(avatarUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 从响应中读取图像数据
	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// 解码图像数据
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return err
	}

	// 将原始图像数据保存为文件
	avatarPath := fmt.Sprintf("./avatar/%s.jpg", openID)
	avatarFileName := fmt.Sprintf("%s.jpg", openID)
	err = os.WriteFile(avatarPath, imgData, 0644)
	if err != nil {
		return err
	}

	// 应用高斯模糊
	blurredImg := imaging.Blur(img, 15) // 调整sigma参数以控制模糊的强度

	// 保存模糊后的图像
	blurredAvatarPath := fmt.Sprintf("./avatar/%s_blur.jpg", openID)
	blurredAvatarFileName := fmt.Sprintf("%s_blur.jpg", openID)
	outFile, err := os.Create(blurredAvatarPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	jpeg.Encode(outFile, blurredImg, &jpeg.Options{Quality: 95}) // 以高质量保存

	// 缩放图像到指定的blockSize
	scaledImg := imaging.Resize(img, blockSize, blockSize, imaging.Lanczos)

	// 保存缩小后的图像
	smallAvatarPath := fmt.Sprintf("./avatar/%s_small.jpg", openID)
	smallAvatarFileName := fmt.Sprintf("%s_small.jpg", openID)
	smallOutFile, err := os.Create(smallAvatarPath)
	if err != nil {
		return err
	}
	defer smallOutFile.Close()

	jpeg.Encode(smallOutFile, scaledImg, &jpeg.Options{Quality: 95}) // 以高质量保存

	// 应用缩小的高斯模糊
	blurredImgSmall := imaging.Blur(scaledImg, 15) // 调整sigma参数以控制模糊的强度

	// 保存缩小模糊后的图像
	blurredSmallAvatarPath := fmt.Sprintf("./avatar/%s_blur_small.jpg", openID)
	blurredSmallAvatarFileName := fmt.Sprintf("%s_blur_small.jpg", openID)
	outFileSmall, err := os.Create(blurredSmallAvatarPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	jpeg.Encode(outFileSmall, blurredImgSmall, &jpeg.Options{Quality: 95}) // 以高质量保存

	// 更新全局avatars数组
	memimg.AvatarsMutex.Lock()
	memimg.Avatars[avatarFileName] = img
	memimg.Avatars[blurredAvatarFileName] = blurredImg
	memimg.Avatars[smallAvatarFileName] = scaledImg
	memimg.Avatars[blurredSmallAvatarFileName] = blurredImgSmall
	memimg.AvatarsMutex.Unlock()

	return nil
}

func UpdateGameMapIfNeeded(game *structs.Game, openID string) ([]structs.Position, error) {
	currentTime := time.Now().Unix()
	elapsed := currentTime - game.LastRefresh
	fmt.Printf("elapsed[%v] game.LastRefresh[%v]\n", elapsed, game.LastRefresh)

	// 计算应该执行的移动次数
	moveInterval := int64(game.RefreshInterval) // 移动间隔，以秒为单位
	moveCount := elapsed / moveInterval
	if moveCount == 0 {
		fmt.Printf("没有到刷新时间,openID[%v]\n", openID)
	} else {
		fmt.Printf("移动次数[%v]\n", moveCount)
	}

	// 检查并添加新蛇
	if _, exists := game.Map.Snakes[openID]; !exists {
		newPos := GenerateRandomPositionWithAvatar(game.Map.Width, game.Map.Height, fmt.Sprintf("%s_small.jpg", openID)) //小头像

		// 随机选择一个方向
		directions := []string{"up", "down", "left", "right"}
		randomDirection := directions[rand.Intn(len(directions))] // 随机选择一个索引

		// 创建并添加新蛇
		game.Map.Snakes[openID] = structs.Snake{
			Positions: []structs.Position{newPos},
			OpenID:    openID,
			Direction: randomDirection, // 使用随机方向
		}
		moveCount = 1
	}

	//没有到刷新时间
	if moveCount == 0 {
		return nil, nil
	}

	// 初始化存放所有被吃掉的食物位置的数组
	allEatenFoodPositions := []structs.Position{}

	// 如果计数可以被5整除，则清理toDelete
	if collisionCheckCount%5 == 0 {
		toDelete = make(map[string]bool)
	}

	// 循环执行移动和碰撞检测
	for i := int64(0); i < moveCount; i++ {
		for id := range game.Map.Snakes {
			// 先检测所有蛇是否咬到自己
			CheckSelfCollision(game.Map.Snakes)

			// 检测碰撞并返回当前被吃掉的食物位置 处理吃掉食物 蛇互相吃掉
			eatenFoodPositions := CheckCollisions(&game.Map)

			// 根据是否吃到食物移动蛇
			game.Map.Snakes[id] = MoveSnake(game.Map.Snakes[id], game.Map.Width, game.Map.Height)

			// 将这次循环中被吃掉的食物位置追加到所有被吃掉的食物位置数组中
			allEatenFoodPositions = append(allEatenFoodPositions, eatenFoodPositions...)

		}
	}

	// 刷新新的时间
	game.LastRefresh = currentTime

	return allEatenFoodPositions, nil
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
	if len(snake.Positions) == 0 {
		return snake
	}

	// 获取头部当前位置
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

	// 创建新头部位置，使用原头部的头像
	newHead := structs.Position{X: newX, Y: newY, Avatar: head.Avatar}

	// 准备一个新的位置列表，首先添加新头部
	newPositions := make([]structs.Position, 1, len(snake.Positions))
	newPositions[0] = newHead

	// 更新头像，头像从原头部开始移动，最后一个头像丢弃
	for i := 1; i < len(snake.Positions); i++ {
		newPositions = append(newPositions, structs.Position{
			X:      snake.Positions[i-1].X,
			Y:      snake.Positions[i-1].Y,
			Avatar: snake.Positions[i].Avatar, // 将前一个位置的头像移动到当前位置
		})
	}

	// 更新蛇的位置列表
	snake.Positions = newPositions
	return snake
}

// WrapPosition 确保位置不会超出地图边界
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

// 蛇吃到了自己函数
func CheckSelfCollision(snakes map[string]structs.Snake) {
	for id, snake := range snakes {
		if len(snake.Positions) < 2 {
			// 如果蛇的长度小于2，它不可能咬到自己
			continue
		}
		head := snake.Positions[0]
		// 检查头部是否与身体的其他部分重叠
		for _, bodyPart := range snake.Positions[1:] {
			if head.X == bodyPart.X && head.Y == bodyPart.Y {
				// 发现碰撞，删除这条蛇
				delete(snakes, id)
				break // 退出当前蛇的检查循环
			}
		}
	}
}

// 更新的CheckCollisions函数，确保立即处理toDelete标记
func CheckCollisions(gameMap *structs.GameMap) []structs.Position {
	positionMap := make(map[structs.Position]string)

	// 记录每个蛇身体的位置到蛇的ID，但在检查时排除当前检查蛇的身体
	for id, snake := range gameMap.Snakes {
		if len(snake.Positions) == 0 || toDelete[id] {
			continue // 如果蛇已标记为删除或没有位置信息，跳过这条蛇
		}
		for i, pos := range snake.Positions {
			if i != 0 { // 还是保留跳过蛇头的条件以防止将蛇头位置也放入
				pos.Avatar = ""
				positionMap[pos] = id
			}
		}
	}

	// 检查头部与其他蛇的身体部分是否重叠
	for id, snake := range gameMap.Snakes {
		if len(snake.Positions) == 0 || toDelete[id] {
			continue // 同样，如果蛇已标记为删除或没有位置信息，跳过这条蛇
		}
		head := snake.Positions[0]
		head.Avatar = ""

		// 清除当前蛇的身体位置信息，以防止自身碰撞的误判
		for _, pos := range snake.Positions {
			delete(positionMap, pos)
		}

		// 检查蛇头是否与其他蛇的身体部分重叠
		if otherID, exists := positionMap[head]; exists && otherID != id {
			fmt.Printf("处理碰撞,id%v,otherid%v", id, otherID)
			ResolveCollision(gameMap.Snakes, id, otherID)
		}

		// 检查结束后重新添加当前蛇的身体到positionMap，为后续检查准备
		for i, pos := range snake.Positions {
			if i != 0 {
				positionMap[pos] = id
			}
		}
	}

	// 检查蛇头是否与食物重叠
	eatenFoodPositions := CheckFoodCollisions(gameMap)

	// 删除被吃掉的蛇
	for id := range toDelete {
		delete(gameMap.Snakes, id)
	}

	return eatenFoodPositions
}

func ResolveCollision(snakes map[string]structs.Snake, snake1ID, snake2ID string) {
	if toDelete[snake1ID] || toDelete[snake2ID] {
		// 如果其中一条蛇已被标记为删除，则不执行吃操作
		return
	}

	snake1 := snakes[snake1ID]
	snake2 := snakes[snake2ID]

	if len(snake1.Positions) >= len(snake2.Positions) {
		EatSnake(snakes, snake1ID, snake2ID)
	} else {
		EatSnake(snakes, snake2ID, snake1ID)
	}
}

func EatSnake(snakes map[string]structs.Snake, eaterID, eatenID string) {
	eater := snakes[eaterID]
	eaten := snakes[eatenID]

	if len(eaten.Positions) > 0 {
		fmt.Printf("蛇吃掉了蛇\n")
		fmt.Printf("蛇吃掉了蛇,toDelete:%v\n", toDelete)
		lastPosition := eaten.Positions[len(eaten.Positions)-1]
		eater.Positions = append(eater.Positions, lastPosition)
		eater.Positions[len(eater.Positions)-1].Avatar = fmt.Sprintf("%s_blur_small.jpg", eaten.OpenID)
	}

	toDelete[eatenID] = true
	snakes[eaterID] = eater
}

func CheckFoodCollisions(gameMap *structs.GameMap) []structs.Position {
	eatenFoodPositions := []structs.Position{} // 用于存放被吃掉的食物
	foodEaten := make(map[int]bool)            // 标记已被吃掉的食物位置

	for _, snake := range gameMap.Snakes {
		if len(snake.Positions) == 0 { // 检查蛇是否有位置，避免访问空数组
			continue // 如果这条蛇没有任何位置数据，跳过这条蛇
		}
		snakeHead := snake.Positions[0] // 蛇头位置
		for i, foodPos := range gameMap.Food {
			if !foodEaten[i] && snakeHead.X == foodPos.X && snakeHead.Y == foodPos.Y {
				// 蛇吃食物
				EatFood(&snake, foodPos, gameMap.Width, gameMap.Height)
				gameMap.Snakes[snake.OpenID] = snake // 更新蛇的状态
				eatenFoodPositions = append(eatenFoodPositions, foodPos)
				foodEaten[i] = true // 标记食物已被吃掉
			}
		}
	}

	// 重建食物数组，排除被吃掉的食物
	newFoodPositions := []structs.Position{}
	for i, foodPos := range gameMap.Food {
		if !foodEaten[i] {
			newFoodPositions = append(newFoodPositions, foodPos)
		}
	}
	gameMap.Food = newFoodPositions // 更新食物位置列表
	return eatenFoodPositions       // 返回被吃掉的食物位置
}

func EatFood(snake *structs.Snake, foodPos structs.Position, width int, height int) {
	// 将食物作为模糊形式添加到蛇的末尾
	foodPos.Avatar = fmt.Sprintf("%s_blur.png", foodPos.Avatar)
	foodPos.Avatar = strings.ReplaceAll(foodPos.Avatar, "_small.png", "")

	if len(snake.Positions) == 1 {
		// 仅有头部，根据头部方向决定尾部的新位置
		head := snake.Positions[0]
		foodPos.X = head.X // 从头部复制位置开始
		foodPos.Y = head.Y // 从头部复制位置开始
		switch snake.Direction {
		case "up":
			foodPos.Y += 1 // 向下增长
		case "down":
			foodPos.Y -= 1 // 向上增长
		case "left":
			foodPos.X += 1 // 向右增长
		case "right":
			foodPos.X -= 1 // 向左增长
		}
	} else {
		// 蛇长度大于1，添加新尾部
		lastPos := snake.Positions[len(snake.Positions)-1]
		secondLastPos := snake.Positions[len(snake.Positions)-2]
		foodPos.X = lastPos.X
		foodPos.Y = lastPos.Y
		if foodPos.X == secondLastPos.X {
			if foodPos.Y < secondLastPos.Y {
				foodPos.Y -= 1
			} else {
				foodPos.Y += 1
			}
		} else if foodPos.Y == secondLastPos.Y {
			if foodPos.X < secondLastPos.X {
				foodPos.X -= 1
			} else {
				foodPos.X += 1
			}
		}

		foodPos.X, foodPos.Y = WrapPosition(foodPos.X, foodPos.Y, width, height) // 确保新尾部位置在边界内
	}

	snake.Positions = append(snake.Positions, foodPos)
}

// AddFoodToGameMap adds a new food item to the game map
func AddFoodToGameMap(gameMap *structs.Game, foodName string) {
	newFood := structs.Position{
		X:      0,
		Y:      0,
		Avatar: foodName + "_small.png",
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
