import os
import click
from flask import Flask, render_template, redirect, url_for, flash, request, jsonify, abort
from flask_login import (
    LoginManager, login_user, current_user, login_required, logout_user
)
from flask_wtf import FlaskForm
from wtforms import StringField, PasswordField, SubmitField
from wtforms.validators import DataRequired, Email, Length, EqualTo
from flask_migrate import Migrate

from models import db, User, Course, Module, Block

from pathlib import Path
from markdown import markdown as md


def create_app():
    app = Flask(__name__, template_folder="templates", static_folder="static")

    @app.template_filter("md")
    def markdown_filter(text):
        return md(text or "", extensions=["extra", "tables", "fenced_code"])

    # --- Конфиг ---
    app.config.setdefault("ALLOW_INIT_DEMO", True)
    app.config["MAX_CONTENT_LENGTH"] = 20 * 1024 * 1024  # 20 MB
    app.config["SUBMISSIONS_REL_PATH"] = "uploads/submissions"
    (Path(app.root_path) / "static" / app.config["SUBMISSIONS_REL_PATH"]).mkdir(
        parents=True, exist_ok=True
    )

    # для картинок внутри текстовых блоков
    app.config["CONTENT_IMAGES_REL_PATH"] = "uploads/content"
    (Path(app.root_path) / "static" / app.config["CONTENT_IMAGES_REL_PATH"]).mkdir(
        parents=True, exist_ok=True
    )

    app.config["SECRET_KEY"] = os.getenv("SECRET_KEY", "secretkey")
    app.config["SQLALCHEMY_DATABASE_URI"] = os.getenv(
        "DATABASE_URL",
        "postgresql+psycopg2://testuser:testpass@db:5432/tester"
    )
    app.config["SQLALCHEMY_TRACK_MODIFICATIONS"] = False

    # --- Инициализация ---
    db.init_app(app)
    Migrate(app, db)  # без db.create_all() тут

    login_manager = LoginManager(app)
    login_manager.login_view = "login"

    @login_manager.user_loader
    def load_user(user_id):
        # На случай, если таблиц ещё нет / миграции не применены
        try:
            return db.session.get(User, int(user_id))
        except Exception:
            db.session.rollback()
            return None

    # --- Формы ---
    class RegisterForm(FlaskForm):
        email = StringField("Email", validators=[DataRequired(), Email(), Length(max=255)])
        password = PasswordField("Пароль", validators=[DataRequired(), Length(min=6, max=64)])
        password2 = PasswordField("Повторите пароль", validators=[DataRequired(), EqualTo("password")])
        submit = SubmitField("Зарегистрироваться")

    class LoginForm(FlaskForm):
        email = StringField("Email", validators=[DataRequired(), Email(), Length(max=255)])
        password = PasswordField("Пароль", validators=[DataRequired(), Length(min=6, max=64)])
        submit = SubmitField("Войти")

    # --- Роуты ---
    @app.route("/")
    def index():
        return render_template("index.html")

    @app.route("/register", methods=["GET", "POST"])
    def register():
        if current_user.is_authenticated:
            return redirect(url_for("dashboard"))
        form = RegisterForm()
        if form.validate_on_submit():
            email = form.email.data.lower().strip()
            if db.session.query(User).filter_by(email=email).first():
                flash("Пользователь с таким email уже зарегистрирован", "warning")
                return render_template("register.html", form=form)
            user = User(email=email)
            user.set_password(form.password.data)
            db.session.add(user)
            db.session.commit()
            login_user(user)
            flash("Регистрация выполнена", "success")
            return redirect(url_for("dashboard"))
        return render_template("register.html", form=form)

    @app.route("/login", methods=["GET", "POST"])
    def login():
        if current_user.is_authenticated:
            return redirect(url_for("dashboard"))
        form = LoginForm()
        if form.validate_on_submit():
            email = form.email.data.lower().strip()
            user = db.session.query(User).filter_by(email=email).first()
            if not user or not user.check_password(form.password.data):
                flash("Неверный email или пароль", "danger")
                return render_template("login.html", form=form)
            login_user(user)
            flash("Вы вошли в аккаунт", "success")
            return redirect(url_for("dashboard"))
        return render_template("login.html", form=form)

    @app.route("/logout")
    @login_required
    def logout():
        logout_user()
        flash("Вы вышли из аккаунта", "info")
        return redirect(url_for("index"))

    @app.route("/dashboard")
    @login_required
    def dashboard():
        return render_template("dashboard.html")

    @app.get("/init-demo")
    def init_demo():
        # Защита: выключи в проде через ALLOW_INIT_DEMO=False
        if not app.config.get("ALLOW_INIT_DEMO", True):
            abort(404)

        # ?reset=1 — опционально дропнуть всё (для локалки)
        do_reset = request.args.get("reset") == "1"

        from models import (
            db, User, Course, Module, Block,
            QuizQuestion, QuizOption
        )

        if do_reset:
            db.drop_all()
        db.create_all()

        # --- пользователи ---
        admin = User.query.filter_by(email="admin@example.com").first()
        if not admin:
            admin = User(email="admin@example.com", role="admin")
            admin.set_password("admin123")
            db.session.add(admin)

        student = User.query.filter_by(email="student@example.com").first()
        if not student:
            student = User(email="student@example.com", role="student")
            student.set_password("student123")
            db.session.add(student)

        # --- курс/модуль/блоки ---
        course = Course.query.filter_by(title="Python для начинающих").first()
        if not course:
            course = Course(
                title="Python для начинающих",
                description="Демо-курс: текст, видео, тест и задание.",
                is_published=True,
            )
            db.session.add(course)
            db.session.flush()

            m1 = Module(course_id=course.id, title="Модуль 1. Старт", order=1)
            db.session.add(m1)
            db.session.flush()

            # text
            b_text = Block(
                module_id=m1.id, type="text", order=1,
                payload={
                    "title": "Краткий гайд по синтаксису",
                    "text": "### Привет!\n\nЭто **демо-текст**.\n\n```python\nprint('hello')\n```"
                }
            )
            db.session.add(b_text)

            # video (embed)
            b_video = Block(
                module_id=m1.id, type="video", order=2,
                payload={
                    "title": "Вступительное видео",
                    "url": "https://www.youtube.com/embed/dQw4w9WgXcQ",
                    "src": "",
                    "caption": "Демо-видео, замени на своё."
                }
            )
            db.session.add(b_video)

            # quiz
            b_quiz = Block(
                module_id=m1.id, type="quiz", order=3,
                payload={"title": "Тест по синтаксису", "pass_score": 70, "require_pass": True}
            )
            db.session.add(b_quiz)
            db.session.flush()

            q1 = QuizQuestion(block_id=b_quiz.id, text="Как оформить цикл в Python?", order=1)
            q2 = QuizQuestion(block_id=b_quiz.id, text="Какая ОС предустановлена с Python?", order=2)
            db.session.add_all([q1, q2])
            db.session.flush()

            db.session.add_all([
                QuizOption(question_id=q1.id, text="for i in range(10):", is_correct=True),
                QuizOption(question_id=q1.id, text="for (i=0; i<10; i++)", is_correct=False),
                QuizOption(question_id=q1.id, text="loop i in range(10)", is_correct=False),

                QuizOption(question_id=q2.id, text="macOS", is_correct=False),
                QuizOption(question_id=q2.id, text="Linux", is_correct=True),
                QuizOption(question_id=q2.id, text="Windows", is_correct=False),
            ])

            # assignment
            b_ass = Block(
                module_id=m1.id, type="assignment", order=4,
                payload={
                    "title": "Домашка 1",
                    "instructions": "Напишите скрипт, печатающий чётные числа 0..20, и приложите файл."
                }
            )
            db.session.add(b_ass)

        db.session.commit()
        return jsonify(ok=True, info="Demo ready. admin/admin, student/student, курс создан.")

    # --- Blueprints ---
    from routes_course import course_bp
    from routes_admin import admin_bp
    from routes_submit import submit_bp
    app.register_blueprint(submit_bp)
    app.register_blueprint(course_bp)
    app.register_blueprint(admin_bp)

    # --- CLI-команды ---
    @app.cli.command("create_admin")
    @click.argument("email")
    @click.argument("password")
    def create_admin(email, password):
        if User.query.filter_by(email=email).first():
            click.echo("Пользователь уже существует")
            return
        u = User(email=email, role="admin")
        u.set_password(password)
        db.session.add(u)
        db.session.commit()
        click.echo(f"Админ создан: {email}")

    return app


app = create_app()

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5001, debug=True)
