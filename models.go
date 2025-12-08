// models.go
package main

import (
	"time"

	"gorm.io/datatypes"
)

// ---------- Пользователь ----------

type User struct {
	ID           uint      `gorm:"primaryKey"`
	Email        string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `gorm:"not null"`
	Role         string    `gorm:"type:varchar(20);not null;default:student"`
	FullName     string    `gorm:"type:varchar(255)"`
	CreatedAt    time.Time

	// сюда при желании можно добавить связи с отправками/попытками
	// Submissions []Submission
	// QuizAttempts []QuizAttempt
}

func (u User) IsAdmin() bool {
	return u.Role == "admin"
}

// ---------- Курс / Модуль / Блок ----------

type Course struct {
	ID        uint      `gorm:"primaryKey"`
	Title     string    `gorm:"size:255;not null"`
	ShortDesc string    `gorm:"type:text"`
	Status    string    `gorm:"size:32;not null;default:'draft'"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Modules []Module `gorm:"foreignKey:CourseID"`
}

type Module struct {
	ID        uint      `gorm:"primaryKey"`
	CourseID  uint      `gorm:"index;not null"`
	Title     string    `gorm:"size:255;not null"`
	Order     int       `gorm:"not null;default:1"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Course Course  `gorm:"constraint:OnDelete:CASCADE;"`
	Blocks []Block `gorm:"foreignKey:ModuleID"`
}

type Block struct {
	ID        uint           `gorm:"primaryKey"`
	ModuleID  uint           `gorm:"index;not null"`
	Module    Module         `gorm:"constraint:OnDelete:CASCADE;"`
	Type      string         `gorm:"size:32;not null"`  // "text", "video", "assignment", "quiz"
	Order     int            `gorm:"not null;default:1"`
	Payload   datatypes.JSON `gorm:"type:jsonb"`        // сырой JSON в БД

	// ВСПОМОГАТЕЛЬНОЕ ПОЛЕ ДЛЯ ШАБЛОНОВ (в памяти, в БД НЕ хранится)
	PayloadMap map[string]any `gorm:"-"`

	CreatedAt time.Time
	UpdatedAt time.Time

	QuizQuestions []QuizQuestion `gorm:"foreignKey:BlockID"`
	Submissions   []Submission   `gorm:"foreignKey:BlockID"`
	QuizAttempts  []QuizAttempt  `gorm:"foreignKey:BlockID"`
}


// ---------- Отправки заданий ----------

type Submission struct {
	ID         uint      `gorm:"primaryKey"`
	UserID     uint      `gorm:"index;not null"`
	BlockID    uint      `gorm:"index;not null"`
	OriginalName string
	StoredPath   string
	Mimetype     string
	SizeBytes    int64
	Comment      string `gorm:"type:text"`
	Status       string `gorm:"type:varchar(32);not null;default:'submitted'"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`

	User  User
	Block Block
}

// ---------- Тесты (quiz) ----------

type QuizQuestion struct {
	ID        uint      `gorm:"primaryKey"`
	BlockID   uint      `gorm:"index;not null"`
	Text      string    `gorm:"type:text;not null"`
	Order     int       `gorm:"not null;default:1"`
	CreatedAt time.Time `gorm:"autoCreateTime"`

	Block   Block
	Options []QuizOption `gorm:"foreignKey:QuestionID;constraint:OnDelete:CASCADE"`
}

type QuizOption struct {
	ID         uint      `gorm:"primaryKey"`
	QuestionID uint      `gorm:"index;not null"`
	Text       string    `gorm:"type:text;not null"`
	IsCorrect  bool      `gorm:"not null;default:false"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`

	Question QuizQuestion
}

type QuizAttempt struct {
	ID        uint           `gorm:"primaryKey"`
	UserID    uint           `gorm:"index;not null"`
	BlockID   uint           `gorm:"index;not null"`
	Score     float64        `gorm:"not null"`                 // процент
	Passed    bool           `gorm:"not null;default:false"`   // прошёл/нет
	Details   datatypes.JSON `gorm:"type:jsonb"`               // JSON с деталями ответов
	CreatedAt time.Time      `gorm:"autoCreateTime"`

	User  User
	Block Block
}
