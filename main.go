package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/hoshinonyaruko/snake-in-im/api"
	"github.com/hoshinonyaruko/snake-in-im/config"
	"github.com/hoshinonyaruko/snake-in-im/memimg"
)

func main() {
	EnsureFoldersExist()
	// Initialize the configuration
	config.LoadConfig("./config.json")
	// 载入头像到内存
	memimg.LoadAvatars("./avatar")
	// 加载食物图标
	memimg.LoadFoods("./foods")
	// 获取blockSize
	blockSize := config.GetConfigValue("blocksize").(int)
	// 预处理
	api.PreloadAndScaleFoods("./foods", blockSize)
	// 检测并热更新到内存 加速绘图
	go memimg.WatchFoods("./foods")
	db := api.InitDB()
	router := gin.Default()
	// 处理玩家改变方向
	router.GET("/update-direction", api.UpdateDirection(db))
	// 渲染函数 返回静态地址
	router.GET("/render-map", api.RenderMapHandler(db))
	// 删除地图
	router.GET("/delete-map", api.DeleteMapHandler(db))
	router.Static("/static", "./static") // 静态文件服务
	// 从配置单例读取端口 监听
	router.Run(":" + config.GetConfigValue("port").(string))
}

// EnsureFoldersExists 检查并创建必需的文件夹
func EnsureFoldersExist() {
	folders := []string{"foods", "static", "avatar"}

	for _, folder := range folders {
		if _, err := os.Stat(folder); os.IsNotExist(err) {
			// 文件夹不存在，尝试创建它
			err := os.Mkdir(folder, 0755) // 使用0755权限以确保读写权限
			if err != nil {
				// 如果创建失败，则记录错误并可能退出程序
				log.Fatalf("Failed to create %s directory: %s", folder, err)
			}
			log.Printf("Created %s directory", folder)
		} else {
			// 文件夹已存在
			log.Printf("%s directory already exists", folder)
		}
	}
}
