package structs

// Position 描述游戏地图上的一个坐标位置。
type Position struct {
	X      int    `json:"x"`      // X坐标
	Y      int    `json:"y"`      // Y坐标
	Avatar string `json:"avatar"` // 头像的本地路径，每个块可独立
}

// Snake 描述一条贪食蛇的信息。
type Snake struct {
	Positions []Position `json:"positions"` // 蛇身上的每个格子的位置
	OpenID    string     `json:"open_id"`   // 用户标识
	Direction string     `json:"direction"` // 移动方向（"up", "down", "left", "right"）
}

// GameMap 描述整个游戏地图的状态。
type GameMap struct {
	Snakes map[string]Snake `json:"snakes"` // 以OpenID为key的蛇的映射
	Food   []Position       `json:"food"`   // 食物的位置，现在为数组
	Width  int              `json:"width"`  // 地图宽度
	Height int              `json:"height"` // 地图高度
}

// Game 描述一个游戏实例，包括组ID和地图状态。
type Game struct {
	GroupID         string  `json:"group_id"`         // 游戏组标识
	Map             GameMap `json:"map"`              // 游戏地图状态
	LastRefresh     int64   `json:"last_refresh"`     // 最后刷新时间，时间戳
	RefreshInterval int     `json:"refresh_interval"` // 刷新间隔，单位秒
}
