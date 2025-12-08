// routes_submit.go
package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	UploadsDir = "static/uploads/submissions"
)

func registerSubmitRoutes(r *gin.Engine) {
	grp := r.Group("/submit")
	grp.POST("/:blockID", authRequired(), submitAssignmentHandler)
}

func submitAssignmentHandler(c *gin.Context) {
	user := getCurrentUser(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login")
		return
	}

	blockIDStr := c.Param("blockID")
	blockID, err := strconv.Atoi(blockIDStr)
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID блока")
		return
	}

	var block Block
	if err := db.First(&block, blockID).Error; err != nil {
		c.String(http.StatusNotFound, "Блок не найден")
		return
	}
	if block.Type != "assignment" {
		c.String(http.StatusBadRequest, "Блок не является заданием")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "Файл обязателен")
		return
	}

	if err := os.MkdirAll(UploadsDir, 0o755); err != nil {
		c.String(http.StatusInternalServerError, "Ошибка создания директории")
		return
	}

	ext := filepath.Ext(file.Filename)
	filename := strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.Itoa(int(user.ID)) + ext
	fullPath := filepath.Join(UploadsDir, filename)

	if err := c.SaveUploadedFile(file, fullPath); err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения файла")
		return
	}

	sub := Submission{
		UserID:       user.ID,
		BlockID:      block.ID,
		OriginalName: file.Filename,
		StoredPath:   fullPath,
		Mimetype:     file.Header.Get("Content-Type"),
		SizeBytes:    file.Size,
		Status:       "submitted",
	}
	if err := db.Create(&sub).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения отправки")
		return
	}

	// редирект обратно на курс с якорем блока
	// нужно найти курс по модулю
	var module Module
	if err := db.First(&module, block.ModuleID).Error; err != nil {
		c.Redirect(http.StatusFound, "/courses")
		return
	}

	c.Redirect(http.StatusFound, "/courses/"+strconv.Itoa(int(module.CourseID))+"#block-"+blockIDStr)
}
