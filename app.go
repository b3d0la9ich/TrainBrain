// app.go
package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	db *gorm.DB

	tmplFuncs = template.FuncMap{
		// аналог |safe
		"safe": func(v any) template.HTML {
			switch x := v.(type) {
			case template.HTML:
				return x
			case string:
				return template.HTML(x)
			default:
				return template.HTML(fmt.Sprint(x))
			}
		},

		// очень простой markdown → HTML
		"md": func(s string) template.HTML {
			if s == "" {
				return ""
			}
			return template.HTML("<p>" + template.HTMLEscapeString(s) + "</p>")
		},

		// a + b
		"add": func(a, b int) int {
			return a + b
		},

		// обрезка строки
		"truncate": func(s string, n int) string {
			if n <= 0 || len(s) <= n {
				return s
			}
			if n <= 1 {
				return s[:n]
			}
			return s[:n-1] + "…"
		},

		// bytes → KB
		"divKB": func(v any) int64 {
			switch x := v.(type) {
			case int:
				return int64(x) / 1024
			case int64:
				return x / 1024
			case uint:
				return int64(x) / 1024
			case uint64:
				return int64(x) / 1024
			default:
				return 0
			}
		},

		// поиск варианта ответа по id
		"findOption": func(q QuizQuestion, optID uint) *QuizOption {
			for i := range q.Options {
				if q.Options[i].ID == optID {
					return &q.Options[i]
				}
			}
			return nil
		},
	}
)

// ---------- БД и миграции ----------

func initDB() *gorm.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// для локального запуска без docker-compose
		dsn = "postgresql://testuser:testpass@localhost:5432/tester?sslmode=disable"
	}

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	if err := autoMigrate(gormDB); err != nil {
		log.Fatalf("autoMigrate error: %v", err)
	}

	seedAdmin(gormDB)

	return gormDB
}

func autoMigrate(gormDB *gorm.DB) error {
	return gormDB.AutoMigrate(
		&User{},
		&Course{},
		&Module{},
		&Block{},
		&Submission{},
		&QuizQuestion{},
		&QuizOption{},
		&QuizAttempt{},
	)
}

// авто-создание админа по ADMIN_EMAIL / ADMIN_PASSWORD
func seedAdmin(gormDB *gorm.DB) {
	email := os.Getenv("ADMIN_EMAIL")
	pass := os.Getenv("ADMIN_PASSWORD")

	if email == "" || pass == "" {
		log.Println("seedAdmin: ADMIN_EMAIL/ADMIN_PASSWORD не заданы – пропускаю создание админа")
		return
	}

	var cnt int64
	if err := gormDB.Model(&User{}).Where("email = ?", email).Count(&cnt).Error; err != nil {
		log.Printf("seedAdmin: ошибка проверки существования админа: %v\n", err)
		return
	}
	if cnt > 0 {
		log.Printf("seedAdmin: админ %s уже существует\n", email)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("seedAdmin: ошибка хеша пароля: %v\n", err)
		return
	}

	admin := User{
		Email:        email,
		PasswordHash: string(hash),
		Role:         "admin",
		CreatedAt:    time.Now(),
	}

	if err := gormDB.Create(&admin).Error; err != nil {
		log.Printf("seedAdmin: ошибка создания админа: %v\n", err)
		return
	}

	log.Printf("seedAdmin: создан админ %s\n", email)
}

// ---------- загрузка шаблонов ----------

func mustParseFile(t *template.Template, name, path string) *template.Template {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("load template %s: %v", path, err)
	}
	t2, err := t.New(name).Parse(string(data))
	if err != nil {
		log.Fatalf("parse template %s: %v", path, err)
	}
	return t2
}

func loadTemplates() *template.Template {
	t := template.New("").Funcs(tmplFuncs)

	// базовый шаблон (если ты его используешь в других)
	t = mustParseFile(t, "base.html", "templates/base.html")

	// основные страницы
	t = mustParseFile(t, "index.html", "templates/index.html")
	t = mustParseFile(t, "login.html", "templates/login.html")
	t = mustParseFile(t, "register.html", "templates/register.html")
	t = mustParseFile(t, "dashboard.html", "templates/dashboard.html")
	t = mustParseFile(t, "courses.html", "templates/courses.html")
	t = mustParseFile(t, "course_player.html", "templates/course_player.html")
	t = mustParseFile(t, "view.html", "templates/view.html")

	// админские и блочные шаблоны (там свои define)
	t = template.Must(t.ParseGlob("templates/admin/*.html"))
	t = template.Must(t.ParseGlob("templates/blocks/*.html"))

	return t
}

// ---------- main ----------

func main() {
	db = initDB()

	r := gin.Default()

	// грузим шаблоны вручную и втыкаем в Gin
	tmpl := loadTemplates()
	r.SetHTMLTemplate(tmpl)

	r.Static("/static", "./static")

	// сессии
	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		secret = "supersecretkey"
	}
	store := cookie.NewStore([]byte(secret))
	r.Use(sessions.Sessions("trainbrain_session", store))

	// роуты
	registerAuthRoutes(r)
	registerCourseRoutes(r)
	registerSubmitRoutes(r)
	registerAdminRoutes(r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "5001"
	}
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// ---------- аутентификация ----------

func registerAuthRoutes(r *gin.Engine) {
	// главная — рисуем index.html
	r.GET("/", func(c *gin.Context) {
		user := getCurrentUser(c)
		c.HTML(http.StatusOK, "index.html", gin.H{
			"User": user,
		})
	})

	r.GET("/register", func(c *gin.Context) {
		user := getCurrentUser(c)
		c.HTML(http.StatusOK, "register.html", gin.H{
			"User": user,
		})
	})

	r.POST("/register", func(c *gin.Context) {
		email := c.PostForm("email")
		password := c.PostForm("password")
		password2 := c.PostForm("password2")

		if email == "" || password == "" {
			c.HTML(http.StatusBadRequest, "register.html", gin.H{
				"Error": "Email и пароль обязательны",
			})
			return
		}
		if password != password2 {
			c.HTML(http.StatusBadRequest, "register.html", gin.H{
				"Error": "Пароли не совпадают",
			})
			return
		}

		var count int64
		db.Model(&User{}).Where("email = ?", email).Count(&count)
		if count > 0 {
			c.HTML(http.StatusBadRequest, "register.html", gin.H{
				"Error": "Пользователь с таким email уже существует",
			})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			c.HTML(http.StatusInternalServerError, "register.html", gin.H{
				"Error": "Ошибка сервера",
			})
			return
		}

		user := User{
			Email:        email,
			PasswordHash: string(hash),
			Role:         "student",
			CreatedAt:    time.Now(),
		}
		if err := db.Create(&user).Error; err != nil {
			c.HTML(http.StatusInternalServerError, "register.html", gin.H{
				"Error": "Ошибка сохранения пользователя",
			})
			return
		}

		sess := sessions.Default(c)
		sess.Set("user_id", user.ID)
		_ = sess.Save()

		c.Redirect(http.StatusFound, "/dashboard")
	})

	r.GET("/login", func(c *gin.Context) {
		user := getCurrentUser(c)
		c.HTML(http.StatusOK, "login.html", gin.H{
			"User": user,
		})
	})

	r.POST("/login", func(c *gin.Context) {
		email := c.PostForm("email")
		password := c.PostForm("password")

		var user User
		if err := db.Where("email = ?", email).First(&user).Error; err != nil {
			c.HTML(http.StatusUnauthorized, "login.html", gin.H{
				"Error": "Неверный email или пароль",
			})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			c.HTML(http.StatusUnauthorized, "login.html", gin.H{
				"Error": "Неверный email или пароль",
			})
			return
		}

		sess := sessions.Default(c)
		sess.Set("user_id", user.ID)
		_ = sess.Save()

		c.Redirect(http.StatusFound, "/dashboard")
	})

	r.GET("/logout", func(c *gin.Context) {
		sess := sessions.Default(c)
		sess.Clear()
		_ = sess.Save()
		c.Redirect(http.StatusFound, "/")
	})

	r.GET("/dashboard", authRequired(), func(c *gin.Context) {
		user := getCurrentUser(c)
		c.HTML(http.StatusOK, "dashboard.html", gin.H{
			"User": user,
		})
	})
}

// ---------- helpers ----------

func getCurrentUser(c *gin.Context) *User {
	sess := sessions.Default(c)
	idVal := sess.Get("user_id")
	if idVal == nil {
		return nil
	}

	var id uint
	switch v := idVal.(type) {
	case uint:
		id = v
	case int:
		id = uint(v)
	case int64:
		id = uint(v)
	case float64:
		id = uint(v)
	default:
		return nil
	}

	var user User
	if err := db.First(&user, id).Error; err != nil {
		return nil
	}
	return &user
}

func authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if getCurrentUser(c) == nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

func adminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := getCurrentUser(c)
		if user == nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		if !user.IsAdmin() {
			c.String(http.StatusForbidden, "Forbidden")
			c.Abort()
			return
		}
		c.Next()
	}
}

// helper для отладки
func debugPrint(err error) {
	if err != nil {
		fmt.Println("DEBUG:", err)
	}
}

type Flash struct {
	Kind string // "success" | "warning" | "danger"
	Msg  string
}

func setFlash(c *gin.Context, kind, msg string) {
	sess := sessions.Default(c)
	sess.Set("flash_kind", kind)
	sess.Set("flash_msg", msg)
	_ = sess.Save()
}

func popFlash(c *gin.Context) *Flash {
	sess := sessions.Default(c)
	k, _ := sess.Get("flash_kind").(string)
	m, _ := sess.Get("flash_msg").(string)
	if k == "" || m == "" {
		return nil
	}
	sess.Delete("flash_kind")
	sess.Delete("flash_msg")
	_ = sess.Save()
	return &Flash{Kind: k, Msg: m}
}
