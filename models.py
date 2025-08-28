from datetime import datetime
from flask_sqlalchemy import SQLAlchemy
from flask_login import UserMixin
from werkzeug.security import generate_password_hash, check_password_hash
from sqlalchemy.dialects.postgresql import JSONB

db = SQLAlchemy()


class User(db.Model, UserMixin):
    __tablename__ = "user"
    id = db.Column(db.Integer, primary_key=True)
    email = db.Column(db.String(255), unique=True, nullable=False, index=True)
    password_hash = db.Column(db.String(255), nullable=False)
    role = db.Column(db.String(32), default="student", nullable=False, index=True)
    created_at = db.Column(db.DateTime, default=datetime.utcnow, nullable=False)

    def set_password(self, raw_password: str):
        self.password_hash = generate_password_hash(raw_password)

    def check_password(self, raw_password: str) -> bool:
        return check_password_hash(self.password_hash, raw_password)


class Course(db.Model):
    __tablename__ = "course"
    id = db.Column(db.Integer, primary_key=True)
    title = db.Column(db.String(255), nullable=False, index=True)
    description = db.Column(db.Text, default="")
    is_published = db.Column(db.Boolean, default=False, index=True)
    created_at = db.Column(db.DateTime, default=datetime.utcnow, nullable=False)


class Module(db.Model):
    __tablename__ = "module"
    id = db.Column(db.Integer, primary_key=True)
    course_id = db.Column(db.Integer, db.ForeignKey("course.id", ondelete="CASCADE"), nullable=False, index=True)
    title = db.Column(db.String(255), nullable=False)
    order = db.Column(db.Integer, default=0, nullable=False, index=True)

    course = db.relationship(
        "Course",
        backref=db.backref("modules", order_by="Module.order", cascade="all, delete-orphan"),
    )


class Block(db.Model):
    __tablename__ = "block"
    id = db.Column(db.Integer, primary_key=True)
    module_id = db.Column(db.Integer, db.ForeignKey("module.id", ondelete="CASCADE"), nullable=False, index=True)
    type = db.Column(db.String(32), nullable=False)   # text, video, quiz, assignment
    payload = db.Column(JSONB, default=dict, nullable=False)
    order = db.Column(db.Integer, default=0, nullable=False, index=True)

    module = db.relationship(
        "Module",
        backref=db.backref("blocks", order_by="Block.order", cascade="all, delete-orphan"),
    )


class Submission(db.Model):
    __tablename__ = "submission"

    id = db.Column(db.Integer, primary_key=True)
    user_id = db.Column(db.Integer, db.ForeignKey("user.id", ondelete="CASCADE"), nullable=False, index=True)
    block_id = db.Column(db.Integer, db.ForeignKey("block.id", ondelete="CASCADE"), nullable=False, index=True)

    original_name = db.Column(db.String(255), nullable=False)
    stored_path  = db.Column(db.String(512), nullable=False)  # относительный путь внутри /static
    mimetype     = db.Column(db.String(128), nullable=True)
    size_bytes   = db.Column(db.Integer, nullable=True)

    comment      = db.Column(db.Text, default="")
    status       = db.Column(db.String(32), default="submitted", index=True)  # submitted/checked/accepted/rejected/needs-fix
    created_at   = db.Column(db.DateTime, default=datetime.utcnow, nullable=False)

    user  = db.relationship("User", backref=db.backref("submissions", cascade="all, delete-orphan"))
    block = db.relationship("Block", backref=db.backref("submissions", cascade="all, delete-orphan"))


# --- QUIZ ---
class QuizQuestion(db.Model):
    __tablename__ = "quiz_question"
    id = db.Column(db.Integer, primary_key=True)
    block_id = db.Column(db.Integer, db.ForeignKey('block.id'), nullable=False)
    text = db.Column(db.Text, nullable=False)
    order = db.Column(db.Integer, default=0)

    block = db.relationship(
        'Block',
        backref=db.backref('quiz_questions', order_by="QuizQuestion.order", cascade="all, delete-orphan"),
    )


class QuizOption(db.Model):
    __tablename__ = "quiz_option"
    id = db.Column(db.Integer, primary_key=True)
    question_id = db.Column(db.Integer, db.ForeignKey('quiz_question.id'), nullable=False)
    text = db.Column(db.Text, nullable=False)
    is_correct = db.Column(db.Boolean, default=False)

    question = db.relationship(
        'QuizQuestion',
        backref=db.backref('options', cascade="all, delete-orphan"),
    )


class QuizAttempt(db.Model):
    __tablename__ = "quiz_attempt"
    id = db.Column(db.Integer, primary_key=True)
    user_id = db.Column(db.Integer, db.ForeignKey("user.id"), nullable=False)
    block_id = db.Column(db.Integer, db.ForeignKey("block.id"), nullable=False)
    score = db.Column(db.Integer, default=0)            # проценты 0..100
    passed = db.Column(db.Boolean, default=False)
    submitted_at = db.Column(db.DateTime, default=datetime.utcnow)
    details = db.Column(db.JSON, default={})            # {question_id: chosen_option_id}

    user = db.relationship("User", backref="quiz_attempts")
    block = db.relationship("Block", backref="quiz_attempts")


# --- ONE-CORRECT-OPTION GUARD (индекс + ORM-хук) -----------------
from sqlalchemy import Index, event, select, and_

# Частичный уникальный индекс (PostgreSQL):
# для каждого question_id может быть только ОДНА строка с is_correct = TRUE
Index(
    "uq_quizoption_one_correct",
    QuizOption.question_id,
    unique=True,
    postgresql_where=QuizOption.is_correct.is_(True),
)

@event.listens_for(QuizOption, "before_insert")
@event.listens_for(QuizOption, "before_update")
def _ensure_single_correct(mapper, connection, target: QuizOption):
    """Запрещаем второй правильный вариант у того же вопроса на уровне ORM."""
    if not target.is_correct:
        return
    qid = target.question_id or (target.question.id if target.question else None)
    if not qid:
        return

    t = QuizOption.__table__
    stmt = (
        select(t.c.id)
        .where(
            and_(
                t.c.question_id == qid,
                t.c.is_correct.is_(True),
                t.c.id != (target.id or 0),
            )
        )
        .limit(1)
    )
    if connection.execute(stmt).first():
        # всплывёт в роуте как ValueError → покажем понятный flash
        raise ValueError("У этого вопроса уже есть правильный вариант ответа.")
