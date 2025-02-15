package main

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/extra/bundebug"
	"github.com/uptrace/bun/extra/bunslog"
)

const name = "go-todoapp"

const version = "0.0.0"

var revision = "HEAD"

//go:embed assets
var assets embed.FS

type Task struct {
	bun.BaseModel `bun:"table:Task,alias:t"`

	ID        int64  `bun:"id,pk,autoincrement" json:"id"`
	Text      string `bun:"text,notnull" json:"text"`
	Completed bool   `bun:"completed,default:false" json:"completed"`
}

func main() {
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	bundb := bun.NewDB(db, pgdialect.New())
	bundb.AddQueryHook(
		bundebug.NewQueryHook(
			bundebug.WithVerbose(true),
			bundebug.FromEnv("BUNDEBUG"),
		),
	)
	bundb.AddQueryHook(
		bunslog.NewQueryHook(
			bunslog.WithQueryLogLevel(slog.LevelDebug),
			bunslog.WithSlowQueryLogLevel(slog.LevelWarn),
			bunslog.WithErrorQueryLogLevel(slog.LevelError),
			bunslog.WithSlowQueryThreshold(3*time.Second),
		),
	)
	defer bundb.Close()

	_, err = bundb.NewCreateTable().Model((*Task)(nil)).IfNotExists().Exec(context.Background())
	if err != nil {
		log.Println(err)
		return
	}

	mime.AddExtensionType(".js", "application/javascript")

	e := echo.New()

	e.POST("/tasks", func(c echo.Context) error {
		var task Task
		if err := c.Bind(&task); err != nil {
			c.Logger().Error("Bind: ", err)
			return c.String(http.StatusBadRequest, "Bind: "+err.Error())
		}
		_, err := bundb.NewInsert().Model(&task).Exec(context.Background())
		if err != nil {
			e.Logger.Error(err)
			return c.JSON(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, task)
	})

	e.GET("/tasks", func(c echo.Context) error {
		var tasks []Task
		err := bundb.NewSelect().Model((*Task)(nil)).Order("id").Scan(context.Background(), &tasks)
		if err != nil {
			e.Logger.Error(err)
			return c.JSON(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, tasks)
	})

	e.POST("/tasks/:id", func(c echo.Context) error {
		var task Task
		if err := c.Bind(&task); err != nil {
			c.Logger().Error("Bind: ", err)
			return c.String(http.StatusBadRequest, "Bind: "+err.Error())
		}
		completed := task.Completed
		err := bundb.NewSelect().Model((*Task)(nil)).Where("id = ?", c.Param("id")).Scan(context.Background(), &task)
		if err != nil {
			e.Logger.Error(err)
			return c.JSON(http.StatusInternalServerError, err.Error())
		}
		task.Completed = completed
		result, err := bundb.NewUpdate().Model(&task).Where("id = ?", c.Param("id")).Exec(context.Background())
		if err != nil {
			e.Logger.Error(err)
			return c.JSON(http.StatusInternalServerError, err.Error())
		}
		if num, err := result.RowsAffected(); err != nil || num == 0 {
			return c.JSON(http.StatusInternalServerError, "No records updated")
		}
		return c.JSON(http.StatusOK, task)
	})

	e.DELETE("/tasks/:id", func(c echo.Context) error {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			return c.String(http.StatusBadRequest, err.Error())
		}
		_, err = bundb.NewDelete().Model((*Task)(nil)).Where(`"id" = ?`, id).Exec(context.Background())
		if err != nil {
			return c.String(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusOK, id)
	})
	e.GET("/tasks/:id", func(c echo.Context) error {
		var task Task
		err := bundb.NewSelect().Model((*Task)(nil)).Where("id = ?", c.Param("id")).Scan(context.Background(), &task)
		if err != nil {
			e.Logger.Error(err)
			return c.JSON(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, task)
	})

	sub, _ := fs.Sub(assets, "assets")
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(sub))))
	e.Logger.Fatal(e.Start(":8989"))
}
