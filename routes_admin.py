from functools import wraps
from flask import Blueprint, render_template, redirect, url_for, request, flash, abort
from flask_login import login_required, current_user
from flask_wtf import FlaskForm
from wtforms import StringField, TextAreaField, SelectField, SubmitField
from wtforms.validators import DataRequired, Length
from sqlalchemy import func
from sqlalchemy.exc import IntegrityError

from models import (
    db, User, Course, Module, Block,
    Submission,
    QuizQuestion, QuizOption
)

admin_bp = Blueprint("admin", __name__, url_prefix="/admin")

SUBMISSION_STATUSES = ["submitted", "checked", "accepted", "rejected", "needs-fix"]

def admin_required(func_):
    @wraps(func_)
    def wrapper(*args, **kwargs):
        if not current_user.is_authenticated:
            return redirect(url_for("login"))
        if getattr(current_user, "role", "student") != "admin":
            abort(403)
        return func_(*args, **kwargs)
    return wrapper

# ---------- Forms ----------
class CourseForm(FlaskForm):
    title = StringField("Название курса", validators=[DataRequired(), Length(max=255)])
    description = TextAreaField("Описание")
    is_published = SelectField("Статус", choices=[("false", "Черновик"), ("true", "Опубликован")])
    submit = SubmitField("Сохранить")

class ModuleForm(FlaskForm):
    title = StringField("Название модуля", validators=[DataRequired(), Length(max=255)])
    submit = SubmitField("Сохранить")

class BlockForm(FlaskForm):
    type = SelectField("Тип блока", choices=[
        ("text", "Текст"),
        ("video", "Видео"),
        ("quiz", "Тест"),
        ("assignment", "Задание"),
    ])
    title = StringField("Заголовок (для text/video/assignment)", validators=[Length(max=255)])
    # text (Markdown/plain)
    text = TextAreaField("Текст (Markdown или обычный)")
    # video
    url = StringField("URL (для видео-embed)")
    src = StringField("MP4 src (для видео)")
    caption = StringField("Подпись")
    # assignment
    instructions = TextAreaField("Инструкции (для assignment)")
    # quiz
    pass_score = SelectField("Проходной балл", choices=[("50","50%"),("60","60%"),("70","70%"),("80","80%"),("90","90%")], default="70")
    require_pass = SelectField("Требовать прохождение перед заданием?", choices=[("true","Да"),("false","Нет")], default="true")

    submit = SubmitField("Сохранить")

# ---------- Courses ----------
@admin_bp.route("/courses")
@login_required
@admin_required
def courses_list():
    courses = Course.query.order_by(Course.created_at.desc()).all()
    return render_template("admin/courses_list.html", courses=courses)

@admin_bp.route("/courses/new", methods=["GET", "POST"])
@login_required
@admin_required
def course_new():
    form = CourseForm()
    if form.validate_on_submit():
        c = Course(
            title=form.title.data.strip(),
            description=form.description.data or "",
            is_published=(form.is_published.data == "true"),
        )
        db.session.add(c)
        db.session.commit()
        flash("Курс создан", "success")
        return redirect(url_for("admin.course_edit", course_id=c.id))
    return render_template("admin/course_form.html", form=form, title="Новый курс")

@admin_bp.route("/courses/<int:course_id>/edit", methods=["GET", "POST"])
@login_required
@admin_required
def course_edit(course_id):
    c = Course.query.get_or_404(course_id)
    form = CourseForm(obj=c)
    if form.validate_on_submit():
        c.title = form.title.data.strip()
        c.description = form.description.data or ""
        c.is_published = (form.is_published.data == "true")
        db.session.commit()
        flash("Курс сохранён", "success")
        return redirect(url_for("admin.course_edit", course_id=c.id))
    form.is_published.data = "true" if c.is_published else "false"
    return render_template("admin/course_form.html", form=form, title=f"Редактирование: {c.title}", course=c)

@admin_bp.route("/courses/<int:course_id>/delete", methods=["POST"])
@login_required
@admin_required
def course_delete(course_id):
    c = Course.query.get_or_404(course_id)
    db.session.delete(c)
    db.session.commit()
    flash("Курс удалён", "info")
    return redirect(url_for("admin.courses_list"))

# ---------- Modules ----------
@admin_bp.route("/courses/<int:course_id>/modules/new", methods=["GET", "POST"])
@login_required
@admin_required
def module_new(course_id):
    c = Course.query.get_or_404(course_id)
    form = ModuleForm()
    if form.validate_on_submit():
        max_order = db.session.query(func.max(Module.order)).filter_by(course_id=c.id).scalar()
        next_order = (max_order or 0) + 1
        m = Module(course=c, title=form.title.data.strip(), order=next_order)
        db.session.add(m)
        db.session.commit()
        flash(f"Модуль создан (№{next_order})", "success")
        return redirect(url_for("admin.course_edit", course_id=c.id))
    return render_template("admin/module_form.html", form=form, course=c, title="Новый модуль")

@admin_bp.route("/modules/<int:module_id>/edit", methods=["GET", "POST"])
@login_required
@admin_required
def module_edit(module_id):
    m = Module.query.get_or_404(module_id)
    form = ModuleForm(obj=m)
    if form.validate_on_submit():
        m.title = form.title.data.strip()
        db.session.commit()
        flash("Модуль сохранён", "success")
        return redirect(url_for("admin.course_edit", course_id=m.course_id))
    return render_template("admin/module_form.html", form=form, course=m.course, title=f"Редактирование: {m.title}")

@admin_bp.route("/modules/<int:module_id>/delete", methods=["POST"])
@login_required
@admin_required
def module_delete(module_id):
    m = Module.query.get_or_404(module_id)
    cid = m.course_id
    db.session.delete(m)
    db.session.commit()
    flash("Модуль удалён", "info")
    return redirect(url_for("admin.course_edit", course_id=cid))

# ---------- Blocks ----------
@admin_bp.route("/modules/<int:module_id>/blocks/new", methods=["GET", "POST"])
@login_required
@admin_required
def block_new(module_id):
    m = Module.query.get_or_404(module_id)
    form = BlockForm()
    if form.validate_on_submit():
        if form.type.data != "video" and (form.url.data or form.src.data):
            form.type.data = "video"

        max_order = db.session.query(func.max(Block.order)).filter_by(module_id=m.id).scalar()
        next_order = (max_order or 0) + 1

        if form.type.data == "text":
            payload = {"title": form.title.data or "", "text": form.text.data or ""}
        elif form.type.data == "video":
            payload = {"title": form.title.data or "", "url": form.url.data or "",
                       "src": form.src.data or "", "caption": form.caption.data or ""}
        elif form.type.data == "assignment":
            payload = {"title": form.title.data or "", "instructions": form.instructions.data or ""}
        elif form.type.data == "quiz":
            payload = {"title": form.title.data or "Тест",
                       "pass_score": int(form.pass_score.data or 70),
                       "require_pass": (form.require_pass.data == "true")}
        else:
            payload = {"title": form.title.data or "Тест"}

        b = Block(module=m, type=form.type.data, order=next_order, payload=payload)
        db.session.add(b)
        db.session.commit()
        flash(f"Блок создан (№{next_order})", "success")
        return redirect(url_for("admin.course_edit", course_id=m.course_id))
    return render_template("admin/block_form.html", form=form, module=m, title="Новый блок")

@admin_bp.route("/blocks/<int:block_id>/edit", methods=["GET", "POST"])
@login_required
@admin_required
def block_edit(block_id):
    b = Block.query.get_or_404(block_id)
    form = BlockForm()
    if request.method == "GET":
        form.type.data = b.type
        form.title.data = b.payload.get("title", "")
        form.text.data = b.payload.get("text", "") or b.payload.get("html", "")
        form.url.data = b.payload.get("url", "")
        form.src.data = b.payload.get("src", "")
        form.caption.data = b.payload.get("caption", "")
        form.instructions.data = b.payload.get("instructions", "")
        form.pass_score.data = str(b.payload.get("pass_score", 70))
        form.require_pass.data = "true" if b.payload.get("require_pass", True) else "false"

    if form.validate_on_submit():
        if form.type.data != "video" and (form.url.data or form.src.data):
            form.type.data = "video"

        b.type = form.type.data
        if b.type == "text":
            b.payload = {"title": form.title.data or "", "text": form.text.data or ""}
        elif b.type == "video":
            b.payload = {"title": form.title.data or "", "url": form.url.data or "",
                         "src": form.src.data or "", "caption": form.caption.data or ""}
        elif b.type == "assignment":
            b.payload = {"title": form.title.data or "", "instructions": form.instructions.data or ""}
        elif b.type == "quiz":
            b.payload = {"title": form.title.data or "Тест",
                         "pass_score": int(form.pass_score.data or 70),
                         "require_pass": (form.require_pass.data == "true")}
        else:
            b.payload = {"title": form.title.data or "Тест"}

        db.session.commit()
        flash("Блок сохранён", "success")
        return redirect(url_for("admin.course_edit", course_id=b.module.course_id))
    return render_template("admin/block_form.html", form=form, module=b.module, title=f"Редактирование блока #{b.id}")

@admin_bp.route("/blocks/<int:block_id>/delete", methods=["POST"])
@login_required
@admin_required
def block_delete(block_id):
    b = Block.query.get_or_404(block_id)
    cid = b.module.course_id
    db.session.delete(b)
    db.session.commit()
    flash("Блок удалён", "info")
    return redirect(url_for("admin.course_edit", course_id=cid))

# ---------- Submissions admin ----------
@admin_bp.route("/submissions")
@login_required
@admin_required
def submissions_list():
    subs = Submission.query.order_by(Submission.created_at.desc()).all()
    return render_template("admin/submissions_list.html", submissions=subs)

@admin_bp.route("/blocks/<int:block_id>/submissions")
@login_required
@admin_required
def submissions_by_block(block_id):
    b = Block.query.get_or_404(block_id)
    subs = Submission.query.filter_by(block_id=block_id).order_by(Submission.created_at.desc()).all()
    return render_template("admin/submissions_list.html", submissions=subs, block=b)

@admin_bp.post("/submissions/<int:submission_id>/update")
@login_required
@admin_required
def submission_update(submission_id: int):
    s = Submission.query.get_or_404(submission_id)
    new_status = (request.form.get("status") or "").strip()
    comment = (request.form.get("comment") or "").strip()
    if new_status not in SUBMISSION_STATUSES:
        flash("Недопустимый статус", "danger")
        return redirect(request.referrer or url_for("admin.submissions_list"))
    s.status = new_status
    s.comment = comment
    db.session.commit()
    flash("Статус отправки обновлён", "success")
    return redirect(request.referrer or url_for("admin.submissions_list"))

# ---------- QUIZ admin ----------
@admin_bp.route("/quizzes/<int:block_id>")
@login_required
@admin_required
def quiz_edit(block_id):
    b = Block.query.get_or_404(block_id)
    if b.type != "quiz":
        abort(404)
    return render_template("admin/quiz_questions.html", block=b)

@admin_bp.route("/quizzes/<int:block_id>/questions/new", methods=["GET","POST"])
@login_required
@admin_required
def quiz_question_new(block_id):
    b = Block.query.get_or_404(block_id)
    if b.type != "quiz":
        abort(404)
    if request.method == "POST":
        text = (request.form.get("text") or "").strip()
        if not text:
            flash("Введите текст вопроса", "warning")
            return redirect(url_for("admin.quiz_question_new", block_id=block_id))
        max_order = db.session.query(func.max(QuizQuestion.order)).filter_by(block_id=b.id).scalar()
        q = QuizQuestion(block=b, text=text, order=(max_order or 0)+1)
        db.session.add(q)
        db.session.commit()
        flash("Вопрос добавлен", "success")
        return redirect(url_for("admin.quiz_edit", block_id=b.id))
    return render_template("admin/quiz_question_form.html", block=b, question=None)

@admin_bp.route("/quizzes/questions/<int:question_id>/delete", methods=["POST"])
@login_required
@admin_required
def quiz_question_delete(question_id):
    q = QuizQuestion.query.get_or_404(question_id)
    block_id = q.block_id
    db.session.delete(q)
    db.session.commit()
    flash("Вопрос удалён", "info")
    return redirect(url_for("admin.quiz_edit", block_id=block_id))

@admin_bp.route("/quizzes/questions/<int:question_id>/options/new", methods=["POST"])
@login_required
@admin_required
def quiz_option_new(question_id):
    q = QuizQuestion.query.get_or_404(question_id)
    text = (request.form.get("text") or "").strip()
    is_correct = (request.form.get("is_correct") == "on")

    if not text:
        flash("Введите текст варианта", "warning")
        return redirect(url_for("admin.quiz_edit", block_id=q.block_id))

    # мягкая проверка до коммита
    if is_correct and QuizOption.query.filter_by(question_id=q.id, is_correct=True).first():
        flash("У этого вопроса уже есть правильный вариант. Снимите флажок с прежнего.", "warning")
        return redirect(url_for("admin.quiz_edit", block_id=q.block_id))

    try:
        db.session.add(QuizOption(question=q, text=text, is_correct=is_correct))
        db.session.commit()
        flash("Вариант добавлен", "success")
    except (IntegrityError, ValueError):
        db.session.rollback()
        flash("У этого вопроса уже есть правильный вариант. Снимите флажок с прежнего.", "warning")
    except Exception as e:
        db.session.rollback()
        flash(f"Ошибка сохранения: {e}", "danger")

    return redirect(url_for("admin.quiz_edit", block_id=q.block_id))

@admin_bp.route("/quizzes/options/<int:option_id>/delete", methods=["POST"])
@login_required
@admin_required
def quiz_option_delete(option_id):
    o = QuizOption.query.get_or_404(option_id)
    block_id = o.question.block_id
    db.session.delete(o)
    db.session.commit()
    flash("Вариант удалён", "info")
    return redirect(url_for("admin.quiz_edit", block_id=block_id))
