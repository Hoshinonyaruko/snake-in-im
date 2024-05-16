package main

import (
	"github.com/gin-gonic/gin"
	"github.com/hoshinonyaruko/snake-in-im/api"
	"github.com/hoshinonyaruko/snake-in-im/config"
	"github.com/hoshinonyaruko/snake-in-im/memimg"
)

func main() {
	// Initialize the configuration
	config.LoadConfig("./config.json")
	// 载入头像到内存
	memimg.LoadAvatars("./avatar")
	go memimg.WatchAvatars("./avatar")
	db := api.InitDB()
	router := gin.Default()
	// 处理玩家改变方向
	router.POST("/update-direction", api.UpdateDirection(db))
	// 渲染函数 返回静态地址
	router.GET("/render-map", api.RenderMapHandler(db))
	router.Static("/static", "./static") // 静态文件服务
	// 从配置单例读取端口 监听
	router.Run(":" + config.GetConfigValue("port"))
}
