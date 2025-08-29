import os
import click
from flask import Flask, render_template, redirect, url_for, flash
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
    app.config["MAX_CONTENT_LENGTH"] = 20 * 1024 * 1024  # 20 MB
    app.config["SUBMISSIONS_REL_PATH"] = "uploads/submissions"
    (Path(app.root_path) / "static" / app.config["SUBMISSIONS_REL_PATH"]).mkdir(parents=True, exist_ok=True)

    app.config["SECRET_KEY"] = os.getenv("SECRET_KEY", "secretkey")
    app.config["SQLALCHEMY_DATABASE_URI"] = os.getenv(
        "DATABASE_URL",
        "postgresql+psycopg2://testuser:testpass@db:5432/tester"
    )
    app.config["SQLALCHEMY_TRACK_MODIFICATIONS"] = False

    # --- Инициализация ---
    db.init_app(app)

    with app.app_context():
        db.create_all()

    Migrate(app, db)

    login_manager = LoginManager(app)
    login_manager.login_view = "login"

    @login_manager.user_loader
    def load_user(user_id):
        return db.session.get(User, int(user_id))

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
