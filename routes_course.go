// routes_course.go
package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func registerCourseRoutes(r *gin.Engine) {
	courseGroup := r.Group("/")
	{
		courseGroup.GET("/courses", listCoursesHandler)
		courseGroup.GET("/courses/:id", viewCourseHandler)
		courseGroup.POST("/courses/:blockID/quiz-submit", authRequired(), submitQuizHandler)
	}
}

// Список курсов — шаблон courses.html
func listCoursesHandler(c *gin.Context) {
	user := getCurrentUser(c)

	var courses []Course
	if err := db.Preload("Modules").
		Order("created_at desc").
		Find(&courses).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка загрузки курсов")
		return
	}

	c.HTML(http.StatusOK, "courses.html", gin.H{
		"User":    user,
		"Courses": courses,
	})
}

// Просмотр курса — шаблон course_player.html
// Просмотр курса — шаблон course_player.html
func viewCourseHandler(c *gin.Context) {
	user := getCurrentUser(c)

	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.String(http.StatusBadRequest, "Некорректный ID курса")
		return
	}

	var course Course
	err = db.Preload("Modules.Blocks", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("blocks.\"order\" asc")
	}).First(&course, id).Error
	if err != nil {
		c.String(http.StatusNotFound, "Курс не найден")
		return
	}

	// Преобразуем Payload → PayloadMap и подгружаем вопросы для квизов
	for mi := range course.Modules {
		for bi := range course.Modules[mi].Blocks {
			block := &course.Modules[mi].Blocks[bi]

			// JSON → map для шаблона
			if len(block.Payload) > 0 {
				var pm map[string]any
				if err := json.Unmarshal(block.Payload, &pm); err == nil {
					block.PayloadMap = pm
				}
			}

			// Для квизов подгружаем вопросы/варианты
			if block.Type == "quiz" {
				var qs []QuizQuestion
				if err := db.Preload("Options").
					Where("block_id = ?", block.ID).
					Find(&qs).Error; err == nil {
					block.QuizQuestions = qs
				}
			}
		}
	}

	c.HTML(http.StatusOK, "course_player.html", gin.H{
		"User":   user,
		"Course": course,
	})
}


// Отправка квиза — считает результат и пишет его в БД, после чего выводит quiz_result.html
func submitQuizHandler(c *gin.Context) {
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
	if block.Type != "quiz" {
		c.String(http.StatusBadRequest, "Этот блок не является тестом")
		return
	}

	var questions []QuizQuestion
	if err := db.Preload("Options").
		Where("block_id = ?", block.ID).
		Find(&questions).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка загрузки вопросов")
		return
	}

	total := len(questions)
	if total == 0 {
		c.String(http.StatusBadRequest, "У теста нет вопросов")
		return
	}

	correctCount := 0
	detailsMap := make(map[string]interface{})

	for _, q := range questions {
		fieldName := "question_" + strconv.Itoa(int(q.ID))
		optIDStr := c.PostForm(fieldName)
		if optIDStr == "" {
			continue
		}
		optID, _ := strconv.Atoi(optIDStr)

		detailsMap[strconv.Itoa(int(q.ID))] = optID

		for _, opt := range q.Options {
			if int(opt.ID) == optID && opt.IsCorrect {
				correctCount++
				break
			}
		}
	}

	score := float64(correctCount) / float64(total) * 100.0

	// Порог прохождения из payload.pass_score (если есть), иначе 60
	passScore := 60.0
	var payload struct {
		PassScore float64 `json:"pass_score"`
	}
	if len(block.Payload) > 0 {
		if err := json.Unmarshal(block.Payload, &payload); err == nil && payload.PassScore > 0 {
			passScore = payload.PassScore
		}
	}

	passed := score >= passScore

	detailsBytes, _ := json.Marshal(detailsMap)

	attempt := QuizAttempt{
		UserID:  user.ID,
		BlockID: block.ID,
		Score:   score,
		Passed:  passed,
		Details: datatypes.JSON(detailsBytes),
	}
	if err := db.Create(&attempt).Error; err != nil {
		c.String(http.StatusInternalServerError, "Ошибка сохранения результата")
		return
	}

	c.HTML(http.StatusOK, "quiz_result.html", gin.H{
		"User":        user,
		"Block":       block,
		"Score":       score,
		"Passed":      passed,
		"PassScore":   passScore,
		"Correct":     correctCount,
		"Total":       total,
		"LastAttempt": attempt,
	})
}
