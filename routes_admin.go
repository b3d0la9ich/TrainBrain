// routes_admin.go
package main

import (
	"encoding/json"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var SubmissionStatuses = []string{
	"submitted", "checked", "accepted", "rejected", "needs-fix",
}

func registerAdminRoutes(r *gin.Engine) {
	admin := r.Group("/admin", authRequired(), adminRequired())
	{
		// DASHBOARD
		admin.GET("/", adminIndexHandler)

		// COURSES
		admin.GET("/courses", adminCoursesListHandler)
		admin.GET("/courses/new", adminCourseNewGetHandler)
		admin.POST("/courses/new", adminCourseNewPostHandler)
		admin.GET("/courses/:course_id/edit", adminCourseEditGetHandler)
		admin.POST("/courses/:course_id/edit", adminCourseEditPostHandler)
		admin.POST("/courses/:course_id/delete", adminCourseDeleteHandler)

		// MODULES
		admin.GET("/courses/:course_id/modules/new", adminModuleNewGetHandler)
		admin.POST("/courses/:course_id/modules/new", adminModuleNewPostHandler)
		admin.GET("/modules/:module_id/edit", adminModuleEditGetHandler)
		admin.POST("/modules/:module_id/edit", adminModuleEditPostHandler)
		admin.POST("/modules/:module_id/delete", adminModuleDeleteHandler)

		// BLOCKS
		admin.GET("/modules/:module_id/blocks/new", adminBlockNewGetHandler)
		admin.POST("/modules/:module_id/blocks/new", adminBlockNewPostHandler)
		admin.GET("/blocks/:block_id/edit", adminBlockEditGetHandler)
		admin.POST("/blocks/:block_id/edit", adminBlockEditPostHandler)
		admin.POST("/blocks/:block_id/delete", adminBlockDeleteHandler)

		// UPLOAD IMAGE
		admin.POST("/uploads/image", adminUploadImageHandler)

		// SUBMISSIONS
		admin.GET("/submissions", adminSubmissionsListHandler)
		admin.GET("/submissions/:submission_id", adminSubmissionViewGetHandler)
		admin.POST("/submissions/:submission_id", adminSubmissionViewPostHandler)
		admin.GET("/blocks/:block_id/submissions", adminSubmissionsByBlockHandler)
		admin.POST("/submissions/:submission_id/delete", adminSubmissionDeleteHandler)

		// QUIZ attempts overview
		admin.GET("/courses/:course_id/quiz-attempts", adminQuizAttemptsHandler)

		// QUIZ admin
		admin.GET("/quizzes/:block_id", adminQuizEditHandler)
		admin.GET("/quizzes/:block_id/questions/new", adminQuizQuestionNewGetHandler)
		admin.POST("/quizzes/:block_id/questions/new", adminQuizQuestionNewPostHandler)
		admin.GET("/quizzes/questions/:question_id/edit", adminQuizQuestionEditGetHandler)
		admin.POST("/quizzes/questions/:question_id/edit", adminQuizQuestionEditPostHandler)
		admin.POST("/quizzes/questions/:question_id/delete", adminQuizQuestionDeleteHandler)
		admin.GET("/quizzes/questions/:question_id/options/new", adminQuizOptionNewGetHandler)
		admin.POST("/quizzes/questions/:question_id/options/new", adminQuizOptionNewPostHandler)
		admin.GET("/quizzes/options/:option_id/edit", adminQuizOptionEditGetHandler)
		admin.POST("/quizzes/options/:option_id/edit", adminQuizOptionEditPostHandler)
		admin.POST("/quizzes/options/:option_id/delete", adminQuizOptionDeleteHandler)
	}
}

///////////////////////////////////////////////////////
// DASHBOARD
///////////////////////////////////////////////////////

func adminIndexHandler(c *gin.Context) {
	var courseCount, usersCount, submissionsCount int64
	db.Model(&Course{}).Count(&courseCount)
	db.Model(&User{}).Count(&usersCount)
	db.Model(&Submission{}).Count(&submissionsCount)

	user := getCurrentUser(c)
	email := ""
	if user != nil {
		email = user.Email
	}

	htmlStr := `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <title>Админ-панель — TrainBrain</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet"
        href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css">
  <link rel="stylesheet"
        href="https://cdn.jsdelivr.net/npm/bootstrap-icons@1.11.3/font/bootstrap-icons.css">
  <link rel="stylesheet" href="/static/css/style.css">
</head>
<body class="bg-light">
<nav class="navbar navbar-expand-lg navbar-dark bg-dark mb-4">
  <div class="container">
    <a class="navbar-brand fw-bold" href="/admin/">TrainBrain Admin</a>
    <div class="ms-auto d-flex gap-2">
      <span class="navbar-text text-light me-3">` + html.EscapeString(email) + `</span>
      <a class="btn btn-outline-light btn-sm" href="/">На сайт</a>
      <a class="btn btn-outline-warning btn-sm" href="/logout">Выйти</a>
    </div>
  </div>
</nav>

<div class="container py-4">
  <h1 class="h3 mb-3">Админ-панель TrainBrain</h1>
  <ul class="list-unstyled mb-4">
    <li>Курсов: ` + strconv.FormatInt(courseCount, 10) + `</li>
    <li>Пользователей: ` + strconv.FormatInt(usersCount, 10) + `</li>
    <li>Отправленных заданий: ` + strconv.FormatInt(submissionsCount, 10) + `</li>
  </ul>

  <div class="d-flex flex-column gap-2">
    <a href="/admin/courses" class="btn btn-primary btn-sm" style="max-width: 260px;">Управление курсами</a>
    <a href="/courses" class="btn btn-outline-secondary btn-sm" style="max-width: 260px;">Список курсов (для пользователей)</a>
    <a href="/" class="btn btn-link btn-sm" style="max-width: 260px;">На главную</a>
  </div>
</div>
</body>
</html>`

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlStr))
}

///////////////////////////////////////////////////////
// COURSES
///////////////////////////////////////////////////////

func adminCoursesListHandler(c *gin.Context) {
	var courses []Course
	if err := db.Order("id").Find(&courses).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка загрузки курсов")
		return
	}
	c.HTML(http.StatusOK, "admin/courses_list.html", gin.H{
		"courses": courses,
		"User":    getCurrentUser(c),
	})
}

func adminCourseNewGetHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "admin/course_form.html", gin.H{
		"course": nil,
		"title":  "Новый курс",
	})
}

func adminCourseNewPostHandler(c *gin.Context) {
	title := strings.TrimSpace(c.PostForm("title"))
	shortDesc := strings.TrimSpace(c.PostForm("short_desc"))
	status := c.PostForm("status")
	if status == "" {
		status = "draft"
	}

	if title == "" {
		c.HTML(http.StatusBadRequest, "admin/course_form.html", gin.H{
			"Error":  "Название курса обязательно",
			"title":  "Новый курс",
			"course": nil,
		})
		return
	}

	course := Course{
		Title:     title,
		ShortDesc: shortDesc,
		Status:    status,
	}
	if err := db.Create(&course).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "admin/course_form.html", gin.H{
			"Error":  "Ошибка сохранения курса",
			"title":  "Новый курс",
			"course": nil,
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/courses/"+strconv.Itoa(int(course.ID))+"/edit")
}

func adminCourseEditGetHandler(c *gin.Context) {
	courseID, err := strconv.Atoi(c.Param("course_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID курса")
		return
	}

	var course Course
	if err := db.
		Preload("Modules", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("\"order\" ASC")
		}).
		Preload("Modules.Blocks", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("\"order\" ASC")
		}).
		First(&course, courseID).Error; err != nil {
		c.String(http.StatusNotFound, "Курс не найден")
		return
	}

	c.HTML(http.StatusOK, "admin/course_form.html", gin.H{
		"course":     course,
		"title":      "Редактирование курса",
		"short_desc": course.ShortDesc,
		"status":     course.Status,
		"modules":    course.Modules,
	})
}

func adminCourseEditPostHandler(c *gin.Context) {
	courseID, err := strconv.Atoi(c.Param("course_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID курса")
		return
	}

	var course Course
	if err := db.First(&course, courseID).Error; err != nil {
		c.String(http.StatusNotFound, "Курс не найден")
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	shortDesc := strings.TrimSpace(c.PostForm("short_desc"))
	status := c.PostForm("status")
	if status == "" {
		status = "draft"
	}

	if title == "" {
		c.HTML(http.StatusBadRequest, "admin/course_form.html", gin.H{
			"Error":      "Название курса обязательно",
			"title":      "Редактирование курса",
			"course":     course,
			"short_desc": shortDesc,
			"status":     status,
		})
		return
	}

	course.Title = title
	course.ShortDesc = shortDesc
	course.Status = status

	if err := db.Save(&course).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "admin/course_form.html", gin.H{
			"Error":      "Ошибка сохранения курса",
			"title":      "Редактирование курса",
			"course":     course,
			"short_desc": shortDesc,
			"status":     status,
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/courses/"+strconv.Itoa(int(course.ID))+"/edit")
}

func adminCourseDeleteHandler(c *gin.Context) {
	courseID, err := strconv.Atoi(c.Param("course_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID курса")
		return
	}
	if err := db.Delete(&Course{}, courseID).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка удаления курса")
		return
	}
	c.Redirect(http.StatusFound, "/admin/courses")
}

///////////////////////////////////////////////////////
// MODULES
///////////////////////////////////////////////////////

func adminModuleNewGetHandler(c *gin.Context) {
	courseID, err := strconv.Atoi(c.Param("course_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID курса")
		return
	}
	var course Course
	if err := db.First(&course, courseID).Error; err != nil {
		c.String(http.StatusNotFound, "Курс не найден")
		return
	}
	c.HTML(http.StatusOK, "admin/module_form.html", gin.H{
		"course": course,
		"title":  "Новый модуль",
	})
}

func adminModuleNewPostHandler(c *gin.Context) {
	courseID, err := strconv.Atoi(c.Param("course_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID курса")
		return
	}
	var course Course
	if err := db.First(&course, courseID).Error; err != nil {
		c.String(http.StatusNotFound, "Курс не найден")
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		c.HTML(http.StatusBadRequest, "admin/module_form.html", gin.H{
			"Error":  "Название модуля обязательно",
			"course": course,
			"title":  "Новый модуль",
		})
		return
	}

	var maxOrder int64
	db.Model(&Module{}).
		Where("course_id = ?", course.ID).
		Select("COALESCE(MAX(\"order\"), 0)").Scan(&maxOrder)

	module := Module{
		Title:    title,
		CourseID: course.ID,
		Order:    int(maxOrder) + 1,
	}
	if err := db.Create(&module).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения модуля")
		return
	}
	c.Redirect(http.StatusFound, "/admin/courses/"+strconv.Itoa(int(course.ID))+"/edit")
}

func adminModuleEditGetHandler(c *gin.Context) {
	moduleID, err := strconv.Atoi(c.Param("module_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID модуля")
		return
	}
	var module Module
	if err := db.Preload("Course").First(&module, moduleID).Error; err != nil {
		c.String(http.StatusNotFound, "Модуль не найден")
		return
	}
	c.HTML(http.StatusOK, "admin/module_form.html", gin.H{
		"course": module.Course,
		"module": module,
		"title":  "Редактирование модуля",
	})
}

func adminModuleEditPostHandler(c *gin.Context) {
	moduleID, err := strconv.Atoi(c.Param("module_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID модуля")
		return
	}
	var module Module
	if err := db.First(&module, moduleID).Error; err != nil {
		c.String(http.StatusNotFound, "Модуль не найден")
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		c.HTML(http.StatusBadRequest, "admin/module_form.html", gin.H{
			"Error":  "Название модуля обязательно",
			"module": module,
			"title":  "Редактирование модуля",
		})
		return
	}

	module.Title = title
	if err := db.Save(&module).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения модуля")
		return
	}

	c.Redirect(http.StatusFound, "/admin/courses/"+strconv.Itoa(int(module.CourseID))+"/edit")
}

func adminModuleDeleteHandler(c *gin.Context) {
	moduleID, err := strconv.Atoi(c.Param("module_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID модуля")
		return
	}
	var module Module
	if err := db.First(&module, moduleID).Error; err != nil {
		c.String(http.StatusNotFound, "Модуль не найден")
		return
	}
	courseID := module.CourseID
	if err := db.Delete(&module).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка удаления модуля")
		return
	}
	c.Redirect(http.StatusFound, "/admin/courses/"+strconv.Itoa(int(courseID))+"/edit")
}

///////////////////////////////////////////////////////
// BLOCKS
///////////////////////////////////////////////////////

func adminBlockNewGetHandler(c *gin.Context) {
	moduleID, err := strconv.Atoi(c.Param("module_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID модуля")
		return
	}
	var module Module
	if err := db.Preload("Course").First(&module, moduleID).Error; err != nil {
		c.String(http.StatusNotFound, "Модуль не найден")
		return
	}

	htmlStr := `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <title>Новый блок — админка</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet"
        href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css">
  <link rel="stylesheet"
        href="https://cdn.jsdelivr.net/npm/bootstrap-icons@1.11.3/font/bootstrap-icons.css">
  <link rel="stylesheet" href="/static/css/style.css">
</head>
<body class="bg-light">

<nav class="navbar navbar-expand-lg navbar-dark bg-dark mb-4">
  <div class="container">
    <a class="navbar-brand fw-bold" href="/admin/">TrainBrain Admin</a>
    <div class="ms-auto d-flex gap-2">
      <a class="btn btn-outline-light btn-sm" href="/">На сайт</a>
      <a class="btn btn-outline-warning btn-sm" href="/logout">Выйти</a>
    </div>
  </div>
</nav>

<div class="container py-4">
  <div class="mb-3">
    <a href="/admin/courses/` + strconv.Itoa(int(module.CourseID)) + `/edit" class="btn btn-link btn-sm">
      &larr; Назад к курсу
    </a>
  </div>

  <h1 class="h4 mb-3">Новый блок для модуля: ` + html.EscapeString(module.Title) + `</h1>

  <form method="post" class="card p-3 shadow-sm">
    <div class="mb-3">
      <label class="form-label">Тип блока</label>
      <select name="type" id="block-type" class="form-select">
        <option value="text">Текст</option>
        <option value="video">Видео</option>
        <option value="assignment">Задание</option>
        <option value="quiz">Тест (quiz)</option>
      </select>
    </div>

    <div class="mb-3">
      <label class="form-label">Заголовок блока</label>
      <input type="text" name="title" class="form-control">
    </div>

    <div class="mb-3 block-field block-text">
      <label class="form-label">Текст</label>
      <textarea name="text" class="form-control" rows="6"></textarea>
    </div>

    <div class="mb-3 block-field block-video" style="display:none;">
      <label class="form-label">Видео</label>

      <div class="mb-2">
        <label class="form-label small text-muted">Внешний URL (YouTube/встраиваемый)</label>
        <input type="url" name="video_url" class="form-control"
               placeholder="https://www.youtube.com/embed/...">
      </div>

      <div>
        <label class="form-label small text-muted">Или файл из uploads/content</label>
        <input type="text" name="video_file" class="form-control"
               placeholder="lesson1.mp4">
        <div class="form-text">
          Файл должен лежать в <code>/static/uploads/content/</code>.
          Введите только имя файла, например <code>lesson1.mp4</code>.
        </div>
      </div>
    </div>

    <div class="mb-3 block-field block-assignment" style="display:none;">
      <label class="form-label">Условие задания</label>
      <textarea name="assignment_prompt" class="form-control" rows="6"></textarea>
    </div>

    <div class="mb-3 block-field block-quiz" style="display:none;">
      <div class="alert alert-info mb-0">
        Это блок теста. Вопросы и варианты ответов можно будет добавить
        после создания блока в разделе &laquo;Тесты&raquo;.
      </div>
    </div>

    <button type="submit" class="btn btn-primary">Сохранить блок</button>
  </form>
</div>

<script>
  function updateBlockFields() {
    var type = document.getElementById('block-type').value;
    document.querySelectorAll('.block-field').forEach(function(el) {
      el.style.display = 'none';
    });
    var target = document.querySelector('.block-' + type);
    if (target) target.style.display = 'block';
  }
  document.getElementById('block-type').addEventListener('change', updateBlockFields);
  updateBlockFields();
</script>

</body>
</html>`

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlStr))
}

func adminBlockNewPostHandler(c *gin.Context) {
	moduleID, err := strconv.Atoi(c.Param("module_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID модуля")
		return
	}
	var module Module
	if err := db.Preload("Course").First(&module, moduleID).Error; err != nil {
		c.String(http.StatusNotFound, "Модуль не найден")
		return
	}

	blockType := c.PostForm("type")
	title := strings.TrimSpace(c.PostForm("title"))
	text := c.PostForm("text")
	videoURL := strings.TrimSpace(c.PostForm("video_url"))
	videoFile := strings.TrimSpace(c.PostForm("video_file"))
	assignmentPrompt := c.PostForm("assignment_prompt")

	var maxOrder int64
	db.Model(&Block{}).
		Where("module_id = ?", module.ID).
		Select("COALESCE(MAX(\"order\"), 0)").Scan(&maxOrder)

	payload := make(map[string]any)
	if title != "" {
		payload["title"] = title
		payload["heading"] = title
		payload["name"] = title
	}

	switch blockType {
	case "text":
		payload["text"] = text
		payload["content"] = text
		payload["body"] = text
		payload["text_md"] = text
	case "video":
		// если указали внешний URL
		if videoURL != "" {
			payload["mode"] = "embed"
			payload["url"] = videoURL
			// для обратной совместимости
			payload["video_url"] = videoURL
		} else if videoFile != "" {
			// локальный файл из static/uploads/content
			if !strings.HasPrefix(videoFile, "/") {
				videoFile = "/static/uploads/content/" + videoFile
			}
			payload["mode"] = "file"
			payload["src"] = videoFile
			// чтобы старый код, если где-то смотрит на video_url, тоже работал
			payload["video_url"] = videoFile
		}
	case "assignment":
		payload["prompt"] = assignmentPrompt
		payload["text"] = assignmentPrompt
		payload["content"] = assignmentPrompt
	}

	payloadBytes, _ := json.Marshal(payload)
	block := Block{
		Type:     blockType,
		ModuleID: module.ID,
		Order:    int(maxOrder) + 1,
		Payload:  datatypes.JSON(payloadBytes),
	}
	if err := db.Create(&block).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения блока")
		return
	}
	c.Redirect(http.StatusFound, "/admin/courses/"+strconv.Itoa(int(module.CourseID))+"/edit")
}

func adminBlockEditGetHandler(c *gin.Context) {
	blockID, err := strconv.Atoi(c.Param("block_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID блока")
		return
	}
	var block Block
	if err := db.Preload("Module").Preload("Module.Course").First(&block, blockID).Error; err != nil {
		c.String(http.StatusNotFound, "Блок не найден")
		return
	}

	payload := map[string]any{}
	_ = json.Unmarshal(block.Payload, &payload)

	ctx := gin.H{
		"module": block.Module,
		"block":  block,
		"title":  "Редактирование блока",
	}

	if v, ok := payload["title"].(string); ok {
		ctx["title_value"] = v
	}
	switch block.Type {
	case "text":
		if v, ok := payload["text"].(string); ok {
			ctx["text"] = v
		}
	case "video":
		// поддерживаем и старый формат (video_url), и новый (mode+url/src)
		if mode, ok := payload["mode"].(string); ok && mode == "file" {
			if v, ok2 := payload["src"].(string); ok2 {
				// в форме можно показывать только имя файла
				ctx["video_file"] = strings.TrimPrefix(v, "/static/uploads/content/")
			}
		} else {
			if v, ok2 := payload["video_url"].(string); ok2 && v != "" {
				ctx["video_url"] = v
			} else if v, ok2 := payload["url"].(string); ok2 {
				ctx["video_url"] = v
			}
		}
	case "assignment":
		if v, ok := payload["prompt"].(string); ok {
			ctx["assignment_prompt"] = v
		}
	}

	c.HTML(http.StatusOK, "admin/block_form.html", ctx)
}

func adminBlockEditPostHandler(c *gin.Context) {
	blockID, err := strconv.Atoi(c.Param("block_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID блока")
		return
	}
	var block Block
	if err := db.Preload("Module").First(&block, blockID).Error; err != nil {
		c.String(http.StatusNotFound, "Блок не найден")
		return
	}

	blockType := c.PostForm("type")
	title := strings.TrimSpace(c.PostForm("title"))
	text := c.PostForm("text")
	videoURL := strings.TrimSpace(c.PostForm("video_url"))
	videoFile := strings.TrimSpace(c.PostForm("video_file"))
	assignmentPrompt := c.PostForm("assignment_prompt")

	payload := make(map[string]any)
	if title != "" {
		payload["title"] = title
		payload["heading"] = title
		payload["name"] = title
	}

	switch blockType {
	case "text":
		payload["text"] = text
		payload["content"] = text
		payload["body"] = text
		payload["text_md"] = text
	case "video":
		if videoURL != "" {
			payload["mode"] = "embed"
			payload["url"] = videoURL
			payload["video_url"] = videoURL
		} else if videoFile != "" {
			if !strings.HasPrefix(videoFile, "/") {
				videoFile = "/static/uploads/content/" + videoFile
			}
			payload["mode"] = "file"
			payload["src"] = videoFile
			payload["video_url"] = videoFile
		}
	case "assignment":
		payload["prompt"] = assignmentPrompt
		payload["text"] = assignmentPrompt
		payload["content"] = assignmentPrompt
	}

	payloadBytes, _ := json.Marshal(payload)
	block.Type = blockType
	block.Payload = datatypes.JSON(payloadBytes)

	if err := db.Save(&block).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения блока")
		return
	}

	c.Redirect(http.StatusFound, "/admin/courses/"+strconv.Itoa(int(block.Module.CourseID))+"/edit")
}

///////////////////////////////////////////////////////
// UPLOAD IMAGE
///////////////////////////////////////////////////////

func adminUploadImageHandler(c *gin.Context) {
	file, err := c.FormFile("image")
	if err != nil || file.Filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Файл не передан"})
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowed := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	}
	if !allowed[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Недопустимый формат файла"})
		return
	}

	safeName := filepath.Base(file.Filename)
	name := time.Now().UTC().Format("20060102150405.000000") + "_" + safeName

	contentRelPath := os.Getenv("CONTENT_IMAGES_REL_PATH")
	if contentRelPath == "" {
		contentRelPath = "uploads/content"
	}
	relPath := filepath.Join(contentRelPath, name)
	absPath := filepath.Join("static", relPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания директории"})
		return
	}

	if err := c.SaveUploadedFile(file, absPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения файла"})
		return
	}

	url := "/static/" + filepath.ToSlash(relPath)
	c.JSON(http.StatusOK, gin.H{"url": url})
}

///////////////////////////////////////////////////////
// SUBMISSIONS
///////////////////////////////////////////////////////

func adminSubmissionsListHandler(c *gin.Context) {
	var subs []Submission
	if err := db.Order("created_at desc").Find(&subs).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка загрузки отправок")
		return
	}
	c.HTML(http.StatusOK, "admin/submissions_list.html", gin.H{
		"submissions": subs,
	})
}

func adminSubmissionViewGetHandler(c *gin.Context) {
	subID, err := strconv.Atoi(c.Param("submission_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID отправки")
		return
	}
	var sub Submission
	if err := db.First(&sub, subID).Error; err != nil {
		c.String(http.StatusNotFound, "Отправка не найдена")
		return
	}

	c.HTML(http.StatusOK, "admin/submission_view.html", gin.H{
		"submission":          sub,
		"submission_statuses": SubmissionStatuses,
	})
}

func adminSubmissionViewPostHandler(c *gin.Context) {
	subID, err := strconv.Atoi(c.Param("submission_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID отправки")
		return
	}
	var sub Submission
	if err := db.First(&sub, subID).Error; err != nil {
		c.String(http.StatusNotFound, "Отправка не найдена")
		return
	}

	sub.Status = c.PostForm("status")
	sub.Comment = c.PostForm("comment")

	if err := db.Save(&sub).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка обновления статуса")
		return
	}

	c.Redirect(http.StatusFound, "/admin/submissions/"+strconv.Itoa(int(sub.ID)))
}

func adminSubmissionsByBlockHandler(c *gin.Context) {
	blockID, err := strconv.Atoi(c.Param("block_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID блока")
		return
	}
	var block Block
	if err := db.First(&block, blockID).Error; err != nil {
		c.String(http.StatusNotFound, "Блок не найден")
		return
	}

	var subs []Submission
	if err := db.Where("block_id = ?", block.ID).
		Order("created_at desc").Find(&subs).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка загрузки отправок")
		return
	}

	c.HTML(http.StatusOK, "admin/submissions_list.html", gin.H{
		"submissions": subs,
		"block":       block,
	})
}

func adminSubmissionDeleteHandler(c *gin.Context) {
	subID, err := strconv.Atoi(c.Param("submission_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID отправки")
		return
	}
	if err := db.Delete(&Submission{}, subID).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка удаления отправки")
		return
	}
	ref := c.Request.Referer()
	if ref == "" {
		ref = "/admin/submissions"
	}
	c.Redirect(http.StatusFound, ref)
}

///////////////////////////////////////////////////////
// QUIZ attempts overview
///////////////////////////////////////////////////////

func adminQuizAttemptsHandler(c *gin.Context) {
	courseID, err := strconv.Atoi(c.Param("course_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID курса")
		return
	}

	var course Course
	if err := db.First(&course, courseID).Error; err != nil {
		c.String(http.StatusNotFound, "Курс не найден")
		return
	}

	var attempts []QuizAttempt
	subq := db.Model(&Block{}).
		Select("blocks.id").
		Joins("JOIN modules m ON m.id = blocks.module_id").
		Where("m.course_id = ?", course.ID)

	if err := db.
		Preload("User").
		Preload("Block").
		Preload("Block.Module").
		Where("block_id IN (?)", subq).
		Order("created_at desc").
		Find(&attempts).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка загрузки попыток")
		return
	}

	c.HTML(http.StatusOK, "admin/quiz_attempts.html", gin.H{
		"course":   course,
		"attempts": attempts,
	})
}

func adminBlockDeleteHandler(c *gin.Context) {
	blockID, err := strconv.Atoi(c.Param("block_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID блока")
		return
	}

	var block Block
	if err := db.Preload("Module").First(&block, blockID).Error; err != nil {
		c.String(http.StatusNotFound, "Блок не найден")
		return
	}

	courseID := block.Module.CourseID

	if err := db.Delete(&block).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка удаления блока")
		return
	}

	c.Redirect(http.StatusFound,
		"/admin/courses/"+strconv.Itoa(int(courseID))+"/edit")
}

///////////////////////////////////////////////////////
// QUIZ admin
///////////////////////////////////////////////////////

func adminQuizEditHandler(c *gin.Context) {
	blockID, err := strconv.Atoi(c.Param("block_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID блока")
		return
	}

	var block Block
	if err := db.Preload("Module").First(&block, blockID).Error; err != nil {
		c.String(http.StatusNotFound, "Блок не найден")
		return
	}
	if block.Type != "quiz" {
		c.Redirect(http.StatusFound,
			"/admin/courses/"+strconv.Itoa(int(block.Module.CourseID))+"/edit")
		return
	}

	// Важное место — подгружаем варианты ответов
	var questions []QuizQuestion
	if err := db.Preload("Options").
		Where("block_id = ?", block.ID).
		Order("id asc").
		Find(&questions).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка загрузки вопросов")
		return
	}

	c.HTML(http.StatusOK, "admin/quiz_questions.html", gin.H{
		"block":     block,
		"questions": questions,
	})
}


func adminQuizQuestionNewGetHandler(c *gin.Context) {
	blockID, err := strconv.Atoi(c.Param("block_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID блока")
		return
	}
	var block Block
	if err := db.First(&block, blockID).Error; err != nil || block.Type != "quiz" {
		c.String(http.StatusNotFound, "Блок не найден или не является тестом")
		return
	}
	c.HTML(http.StatusOK, "admin/quiz_question_form.html", gin.H{
		"block": block,
		"title": "Новый вопрос",
	})
}

func adminQuizQuestionNewPostHandler(c *gin.Context) {
	blockID, err := strconv.Atoi(c.Param("block_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID блока")
		return
	}
	var block Block
	if err := db.First(&block, blockID).Error; err != nil || block.Type != "quiz" {
		c.String(http.StatusNotFound, "Блок не найден или не является тестом")
		return
	}
	text := strings.TrimSpace(c.PostForm("text"))
	if text == "" {
		c.HTML(http.StatusBadRequest, "admin/quiz_question_form.html", gin.H{
			"Error": "Текст вопроса обязателен",
			"block": block,
			"title": "Новый вопрос",
		})
		return
	}
	q := QuizQuestion{
		BlockID: block.ID,
		Text:    text,
	}
	if err := db.Create(&q).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения вопроса")
		return
	}
	c.Redirect(http.StatusFound, "/admin/quizzes/"+strconv.Itoa(int(block.ID)))
}

func adminQuizQuestionEditGetHandler(c *gin.Context) {
	qID, err := strconv.Atoi(c.Param("question_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID вопроса")
		return
	}
	var q QuizQuestion
	if err := db.Preload("Block").First(&q, qID).Error; err != nil {
		c.String(http.StatusNotFound, "Вопрос не найден")
		return
	}
	c.HTML(http.StatusOK, "admin/quiz_question_form.html", gin.H{
		"block": q.Block,
		"title": "Редактирование вопроса",
		"q":     q,
	})
}

func adminQuizQuestionEditPostHandler(c *gin.Context) {
	qID, err := strconv.Atoi(c.Param("question_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID вопроса")
		return
	}
	var q QuizQuestion
	if err := db.Preload("Block").First(&q, qID).Error; err != nil {
		c.String(http.StatusNotFound, "Вопрос не найден")
		return
	}
	text := strings.TrimSpace(c.PostForm("text"))
	if text == "" {
		c.HTML(http.StatusBadRequest, "admin/quiz_question_form.html", gin.H{
			"Error": "Текст вопроса обязателен",
			"block": q.Block,
			"title": "Редактирование вопроса",
			"q":     q,
		})
		return
	}
	q.Text = text
	if err := db.Save(&q).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения вопроса")
		return
	}
	c.Redirect(http.StatusFound, "/admin/quizzes/"+strconv.Itoa(int(q.BlockID)))
}

func adminQuizQuestionDeleteHandler(c *gin.Context) {
	qID, err := strconv.Atoi(c.Param("question_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID вопроса")
		return
	}
	var q QuizQuestion
	if err := db.First(&q, qID).Error; err != nil {
		c.String(http.StatusNotFound, "Вопрос не найден")
		return
	}
	blockID := q.BlockID
	if err := db.Delete(&q).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка удаления вопроса")
		return
	}
	c.Redirect(http.StatusFound, "/admin/quizzes/"+strconv.Itoa(int(blockID)))
}

func adminQuizOptionNewGetHandler(c *gin.Context) {
	qID, err := strconv.Atoi(c.Param("question_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID вопроса")
		return
	}
	var q QuizQuestion
	if err := db.Preload("Block").First(&q, qID).Error; err != nil {
		c.String(http.StatusNotFound, "Вопрос не найден")
		return
	}
	c.HTML(http.StatusOK, "admin/quiz_question_form.html", gin.H{
		"block": q.Block,
		"title": "Новый вариант для вопроса #" + strconv.Itoa(int(q.ID)),
		"q":     q,
	})
}

func adminQuizOptionNewPostHandler(c *gin.Context) {
	qID, err := strconv.Atoi(c.Param("question_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID вопроса")
		return
	}
	var q QuizQuestion
	if err := db.Preload("Block").First(&q, qID).Error; err != nil {
		c.String(http.StatusNotFound, "Вопрос не найден")
		return
	}

	text := strings.TrimSpace(c.PostForm("text"))
	isCorrectVal := c.PostForm("is_correct")
	isCorrect := isCorrectVal == "yes"

	if text == "" {
		c.HTML(http.StatusBadRequest, "admin/quiz_question_form.html", gin.H{
			"Error": "Текст варианта обязателен",
			"block": q.Block,
			"title": "Новый вариант для вопроса #" + strconv.Itoa(int(q.ID)),
			"q":     q,
		})
		return
	}

	if isCorrect {
		var count int64
		db.Model(&QuizOption{}).
			Where("question_id = ? AND is_correct = ?", q.ID, true).
			Count(&count)
		if count > 0 {
			c.HTML(http.StatusBadRequest, "admin/quiz_question_form.html", gin.H{
				"Error": "У этого вопроса уже есть правильный вариант ответа.",
				"block": q.Block,
				"title": "Новый вариант для вопроса #" + strconv.Itoa(int(q.ID)),
				"q":     q,
			})
			return
		}
	}

	opt := QuizOption{
		QuestionID: q.ID,
		Text:       text,
		IsCorrect:  isCorrect,
	}
	if err := db.Create(&opt).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения варианта")
		return
	}

	c.Redirect(http.StatusFound, "/admin/quizzes/"+strconv.Itoa(int(q.BlockID)))
}

func adminQuizOptionEditGetHandler(c *gin.Context) {
	optID, err := strconv.Atoi(c.Param("option_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID варианта")
		return
	}
	var opt QuizOption
	if err := db.Preload("Question").Preload("Question.Block").First(&opt, optID).Error; err != nil {
		c.String(http.StatusNotFound, "Вариант не найден")
		return
	}
	isCorrectStr := "no"
	if opt.IsCorrect {
		isCorrectStr = "yes"
	}
	c.HTML(http.StatusOK, "admin/quiz_question_form.html", gin.H{
		"opt":        opt,
		"is_correct": isCorrectStr,
		"block":      opt.Question.Block,
		"title":      "Редактирование варианта для вопроса #" + strconv.Itoa(int(opt.QuestionID)),
	})
}

func adminQuizOptionEditPostHandler(c *gin.Context) {
	optID, err := strconv.Atoi(c.Param("option_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID варианта")
		return
	}
	var opt QuizOption
	if err := db.Preload("Question").Preload("Question.Block").First(&opt, optID).Error; err != nil {
		c.String(http.StatusNotFound, "Вариант не найден")
		return
	}

	text := strings.TrimSpace(c.PostForm("text"))
	isCorrectVal := c.PostForm("is_correct")
	isCorrect := isCorrectVal == "yes"

	if text == "" {
		c.HTML(http.StatusBadRequest, "admin/quiz_question_form.html", gin.H{
			"Error": "Текст варианта обязателен",
			"opt":   opt,
			"block": opt.Question.Block,
			"title": "Редактирование варианта для вопроса #" + strconv.Itoa(int(opt.QuestionID)),
		})
		return
	}

	if isCorrect && !opt.IsCorrect {
		var count int64
		db.Model(&QuizOption{}).
			Where("question_id = ? AND is_correct = ?", opt.QuestionID, true).
			Count(&count)
		if count > 0 {
			c.HTML(http.StatusBadRequest, "admin/quiz_question_form.html", gin.H{
				"Error": "У этого вопроса уже есть правильный вариант ответа.",
				"opt":   opt,
				"block": opt.Question.Block,
				"title": "Редактирование варианта для вопроса #" + strconv.Itoa(int(opt.QuestionID)),
			})
			return
		}
	}

	opt.Text = text
	opt.IsCorrect = isCorrect

	if err := db.Save(&opt).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения варианта")
		return
	}

	c.Redirect(http.StatusFound, "/admin/quizzes/"+strconv.Itoa(int(opt.Question.BlockID)))
}

func adminQuizOptionDeleteHandler(c *gin.Context) {
	optID, err := strconv.Atoi(c.Param("option_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID варианта")
		return
	}
	var opt QuizOption
	if err := db.Preload("Question").First(&opt, optID).Error; err != nil {
		c.String(http.StatusNotFound, "Вариант не найден")
		return
	}
	blockID := opt.Question.BlockID
	if err := db.Delete(&opt).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка удаления варианта")
		return
	}
	c.Redirect(http.StatusFound, "/admin/quizzes/"+strconv.Itoa(int(blockID)))
}
