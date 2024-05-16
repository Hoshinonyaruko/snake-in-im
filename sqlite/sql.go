package sqlite

import (
	"database/sql"
	"encoding/json"
	"log"

	"github.com/hoshinonyaruko/snake-in-im/structs"
)

const createGamesTableSQL = `
CREATE TABLE IF NOT EXISTS Games (
    GroupID TEXT PRIMARY KEY,
    MapWidth INTEGER,
    MapHeight INTEGER,
    LastRefresh TIMESTAMP,
    RefreshInterval INTEGER
);
`

const createSnakesTableSQL = `
CREATE TABLE IF NOT EXISTS Snakes (
    GroupID TEXT,
    OpenID TEXT,
    Positions TEXT,
    Direction TEXT,
    PRIMARY KEY (GroupID, OpenID)
);
`

const createFoodsTableSQL = `
CREATE TABLE IF NOT EXISTS Foods (
    GroupID TEXT,
    Position TEXT,
    PRIMARY KEY (GroupID, Position)
);
`

const createSnakesIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_snake_group ON Snakes (GroupID);
`

func executeSQL(db *sql.DB, sqlStatement string) {
	_, err := db.Exec(sqlStatement)
	if err != nil {
		log.Fatalf("Error executing SQL statement: %s\n%s", sqlStatement, err)
	}
}

func InitializeDatabase(db *sql.DB) {
	executeSQL(db, createGamesTableSQL)
	executeSQL(db, createSnakesTableSQL)
	executeSQL(db, createFoodsTableSQL)
	executeSQL(db, createSnakesIndexSQL)
}

func UpdateGameMapInDB(db *sql.DB, game *structs.Game) error {
	// 开启事务
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	// 更新游戏基本信息
	_, err = tx.Exec("UPDATE Games SET MapWidth = ?, MapHeight = ?, LastRefresh = ?, RefreshInterval = ? WHERE GroupID = ?",
		game.Map.Width, game.Map.Height, game.LastRefresh, game.RefreshInterval, game.GroupID)
	if err != nil {
		tx.Rollback()
		return err
	}

	// 更新食物位置
	for _, foodPos := range game.Map.Food {
		foodData, err := json.Marshal(foodPos)
		if err != nil {
			tx.Rollback()
			return err
		}
		_, err = tx.Exec("INSERT OR REPLACE INTO Foods (GroupID, Position) VALUES (?, ?)", game.GroupID, string(foodData))
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// 更新所有蛇的信息
	for _, snake := range game.Map.Snakes {
		positionsData, err := json.Marshal(snake.Positions)
		if err != nil {
			tx.Rollback()
			return err
		}
		_, err = tx.Exec("INSERT OR REPLACE INTO Snakes (GroupID, OpenID, Positions, Direction) VALUES (?, ?, ?, ?)",
			game.GroupID, snake.OpenID, string(positionsData), snake.Direction)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// 提交事务
	return tx.Commit()
}
