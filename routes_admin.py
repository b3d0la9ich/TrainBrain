import os
from datetime import datetime
from functools import wraps
from flask import Blueprint, render_template, redirect, url_for, request, flash, abort, current_app, jsonify
from flask_login import login_required, current_user
from flask_wtf import FlaskForm
from wtforms import StringField, TextAreaField, SelectField, SubmitField
from wtforms.validators import DataRequired, Length
from sqlalchemy import func
from sqlalchemy.exc import IntegrityError
from werkzeug.utils import secure_filename

from models import (
    db, User, Course, Module, Block,
    Submission,
    QuizQuestion, QuizOption, QuizAttempt
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


# ---------- FORMS ----------

class CourseForm(FlaskForm):
    title = StringField("Название курса", validators=[DataRequired(), Length(max=255)])
    short_desc = TextAreaField("Краткое описание")
    status = SelectField(
        "Статус",
        choices=[("draft", "Черновик"), ("published", "Опубликован")],
        default="draft",
    )
    submit = SubmitField("Сохранить")


class ModuleForm(FlaskForm):
    title = StringField("Название модуля", validators=[DataRequired(), Length(max=255)])
    order = StringField("Порядок", validators=[DataRequired()])
    submit = SubmitField("Сохранить")


class BlockForm(FlaskForm):
    type = SelectField(
        "Тип блока",
        choices=[
            ("text", "Текст"),
            ("video", "Видео"),
            ("assignment", "Задание"),
            ("quiz", "Тест-квиз"),
        ],
        validators=[DataRequired()],
    )
    title = StringField("Заголовок блока", validators=[Length(max=255)])
    text = TextAreaField("Текст")
    video_url = StringField("URL видео", validators=[Length(max=1024)])
    assignment_prompt = TextAreaField("Условие задания")
    submit = SubmitField("Сохранить")


class SubmissionStatusForm(FlaskForm):
    status = SelectField("Статус", choices=[(s, s) for s in SUBMISSION_STATUSES])
    comment = TextAreaField("Комментарий проверяющего")
    submit = SubmitField("Сохранить")


class QuizQuestionForm(FlaskForm):
    text = TextAreaField("Текст вопроса", validators=[DataRequired()])
    submit = SubmitField("Сохранить")


class QuizOptionForm(FlaskForm):
    text = TextAreaField("Текст варианта", validators=[DataRequired()])
    is_correct = SelectField(
        "Правильный?", choices=[("no", "Нет"), ("yes", "Да")], default="no"
    )
    submit = SubmitField("Сохранить")


# ---------- ADMIN DASHBOARD ----------

@admin_bp.route("/")
@login_required
@admin_required
def admin_index():
    course_count = Course.query.count()
    users_count = User.query.count()
    submissions_count = Submission.query.count()
    return render_template(
        "admin/courses_list.html",
        course_count=course_count,
        users_count=users_count,
        submissions_count=submissions_count,
    )


# ---------- COURSES ----------

@admin_bp.route("/courses")
@login_required
@admin_required
def courses_list():
    courses = Course.query.order_by(Course.id).all()
    return render_template("admin/courses_list.html", courses=courses)


@admin_bp.route("/courses/new", methods=["GET", "POST"])
@login_required
@admin_required
def course_new():
    form = CourseForm()
    if form.validate_on_submit():
        max_order = db.session.query(func.coalesce(func.max(Course.order), 0)).scalar()
        course = Course(
            title=form.title.data.strip(),
            short_desc=form.short_desc.data.strip(),
            status=form.status.data,
            order=max_order + 1,
        )
        db.session.add(course)
        db.session.commit()
        flash("Курс создан", "success")
        return redirect(url_for("admin.course_edit", course_id=course.id))
    return render_template("admin/course_form.html", form=form, course=None, title="Новый курс")


@admin_bp.route("/courses/<int:course_id>/edit", methods=["GET", "POST"])
@login_required
@admin_required
def course_edit(course_id):
    course = Course.query.get_or_404(course_id)
    form = CourseForm(obj=course)
    if form.validate_on_submit():
        course.title = form.title.data.strip()
        course.short_desc = form.short_desc.data.strip()
        course.status = form.status.data
        db.session.commit()
        flash("Курс обновлён", "success")
        return redirect(url_for("admin.course_edit", course_id=course.id))
    return render_template("admin/course_form.html", form=form, course=course, title="Редактирование курса")


@admin_bp.route("/courses/<int:course_id>/delete", methods=["POST"])
@login_required
@admin_required
def course_delete(course_id):
    course = Course.query.get_or_404(course_id)
    db.session.delete(course)
    db.session.commit()
    flash("Курс удалён", "info")
    return redirect(url_for("admin.courses_list"))


# ---------- MODULES ----------

@admin_bp.route("/courses/<int:course_id>/modules/new", methods=["GET", "POST"])
@login_required
@admin_required
def module_new(course_id):
    course = Course.query.get_or_404(course_id)
    form = ModuleForm()
    if form.validate_on_submit():
        max_order = (
            db.session.query(func.coalesce(func.max(Module.order), 0))
            .filter(Module.course_id == course.id)
            .scalar()
        )
        module = Module(
            title=form.title.data.strip(),
            order=int(form.order.data),
            course_id=course.id,
        )
        if module.order <= 0:
            module.order = max_order + 1
        db.session.add(module)
        db.session.commit()
        flash("Модуль создан", "success")
        return redirect(url_for("admin.course_edit", course_id=course.id))
    return render_template("admin/module_form.html", form=form, course=course, title="Новый модуль")


@admin_bp.route("/modules/<int:module_id>/edit", methods=["GET", "POST"])
@login_required
@admin_required
def module_edit(module_id):
    module = Module.query.get_or_404(module_id)
    form = ModuleForm(obj=module)
    if form.validate_on_submit():
        module.title = form.title.data.strip()
        try:
            module.order = int(form.order.data)
        except ValueError:
            flash("Порядок должен быть числом", "warning")
        db.session.commit()
        flash("Модуль обновлён", "success")
        return redirect(url_for("admin.course_edit", course_id=module.course_id))
    return render_template("admin/module_form.html", form=form, course=module.course, title="Редактирование модуля")


@admin_bp.route("/modules/<int:module_id>/delete", methods=["POST"])
@login_required
@admin_required
def module_delete(module_id):
    module = Module.query.get_or_404(module_id)
    course_id = module.course_id
    db.session.delete(module)
    db.session.commit()
    flash("Модуль удалён", "info")
    return redirect(url_for("admin.course_edit", course_id=course_id))


# ---------- BLOCKS ----------

@admin_bp.route("/modules/<int:module_id>/blocks/new", methods=["GET", "POST"])
@login_required
@admin_required
def block_new(module_id):
    module = Module.query.get_or_404(module_id)
    form = BlockForm()
    if form.validate_on_submit():
        max_order = (
            db.session.query(func.coalesce(func.max(Block.order), 0))
            .filter(Block.module_id == module.id)
            .scalar()
        )
        payload = {}
        if form.title.data:
            payload["title"] = form.title.data.strip()
        if form.type.data == "text":
            payload["text"] = form.text.data
        elif form.type.data == "video":
            payload["video_url"] = form.video_url.data.strip()
        elif form.type.data == "assignment":
            payload["prompt"] = form.assignment_prompt.data

        block = Block(
            type=form.type.data,
            module_id=module.id,
            order=max_order + 1,
            payload=payload,
        )
        db.session.add(block)
        db.session.commit()
        flash("Блок создан", "success")
        return redirect(url_for("admin.course_edit", course_id=module.course_id))
    return render_template(
        "admin/block_form.html",
        form=form,
        module=module,
        title="Новый блок",
    )


@admin_bp.route("/blocks/<int:block_id>/edit", methods=["GET", "POST"])
@login_required
@admin_required
def block_edit(block_id):
    block = Block.query.get_or_404(block_id)
    module = block.module
    form = BlockForm()

    if request.method == "GET":
        form.type.data = block.type
        form.title.data = block.payload.get("title", "")
        if block.type == "text":
            form.text.data = block.payload.get("text", "")
        elif block.type == "video":
            form.video_url.data = block.payload.get("video_url", "")
        elif block.type == "assignment":
            form.assignment_prompt.data = block.payload.get("prompt", "")

    if form.validate_on_submit():
        block.type = form.type.data
        payload = {}
        if form.title.data:
            payload["title"] = form.title.data.strip()
        if form.type.data == "text":
            payload["text"] = form.text.data
        elif form.type.data == "video":
            payload["video_url"] = form.video_url.data.strip()
        elif form.type.data == "assignment":
            payload["prompt"] = form.assignment_prompt.data
        block.payload = payload
        db.session.commit()
        flash("Блок обновлён", "success")
        return redirect(url_for("admin.course_edit", course_id=module.course_id))

    return render_template(
        "admin/block_form.html",
        form=form,
        module=module,
        title="Редактирование блока",
    )


@admin_bp.route("/blocks/<int:block_id>/delete", methods=["POST"])
@login_required
@admin_required
def block_delete(block_id):
    block = Block.query.get_or_404(block_id)
    cid = block.module.course_id
    db.session.delete(block)
    db.session.commit()
    flash("Блок удалён", "info")
    return redirect(url_for("admin.course_edit", course_id=cid))


@admin_bp.route("/uploads/image", methods=["POST"])
@login_required
@admin_required
def upload_image():
    """Загрузка изображения для вставки в текстовый блок.

    Возвращает JSON: {"url": "<абсолютный URL>"} или ошибку 4xx/5xx.
    """
    file = request.files.get("image")
    if not file or not file.filename:
        return jsonify({"error": "Файл не передан"}), 400

    _, ext = os.path.splitext(file.filename)
    ext = (ext or "").lower()
    allowed = {".png", ".jpg", ".jpeg", ".gif", ".webp"}
    if ext not in allowed:
        return jsonify({"error": "Недопустимый формат файла"}), 400

    safe = secure_filename(file.filename)
    name = f"{datetime.utcnow().strftime('%Y%m%d%H%M%S%f')}_{safe}"
    rel_path = os.path.join(
        current_app.config.get("CONTENT_IMAGES_REL_PATH", "uploads/content"),
        name,
    )
    abs_path = os.path.join(current_app.static_folder, rel_path)
    os.makedirs(os.path.dirname(abs_path), exist_ok=True)
    file.save(abs_path)

    url = url_for("static", filename=rel_path)
    return jsonify({"url": url})


# ---------- SUBMISSIONS ----------

@admin_bp.route("/submissions")
@login_required
@admin_required
def submissions_list():
    submissions = (
        Submission.query
        .order_by(Submission.created_at.desc())
        .all()
    )
    return render_template("admin/submissions_list.html", submissions=submissions)


@admin_bp.route("/submissions/<int:submission_id>", methods=["GET", "POST"])
@login_required
@admin_required
def submission_view(submission_id):
    submission = Submission.query.get_or_404(submission_id)
    form = SubmissionStatusForm(obj=submission)
    if form.validate_on_submit():
        submission.status = form.status.data
        submission.comment = form.comment.data
        db.session.commit()
        flash("Статус обновлён", "success")
        return redirect(url_for("admin.submission_view", submission_id=submission.id))
    return render_template("admin/submission_view.html", submission=submission, form=form)


@admin_bp.route("/blocks/<int:block_id>/submissions")
@login_required
@admin_required
def submissions_by_block(block_id):
    block = Block.query.get_or_404(block_id)
    submissions = (
        Submission.query
        .filter(Submission.block_id == block.id)
        .order_by(Submission.created_at.desc())
        .all()
    )
    return render_template(
        "admin/submissions_list.html",
        submissions=submissions,
        block=block,
    )


@admin_bp.route("/submissions/<int:submission_id>/delete", methods=["POST"])
@login_required
@admin_required
def submission_delete(submission_id):
    submission = Submission.query.get_or_404(submission_id)
    db.session.delete(submission)
    db.session.commit()
    flash("Отправка удалена", "info")
    return redirect(request.referrer or url_for("admin.submissions_list"))


# ---------- QUIZ attempts overview ----------

@admin_bp.route("/courses/<int:course_id>/quiz-attempts")
@login_required
@admin_required
def quiz_attempts(course_id):
    course = Course.query.get_or_404(course_id)
    attempts = (
        QuizAttempt.query
        .join(QuizAttempt.block)
        .join(Block.module)
        .filter(Module.course_id == course.id)
        .order_by(QuizAttempt.submitted_at.desc())
        .all()
    )
    return render_template("admin/quiz_attempts.html", course=course, attempts=attempts)


# ---------- QUIZ admin ----------

@admin_bp.route("/quizzes/<int:block_id>")
@login_required
@admin_required
def quiz_edit(block_id):
    block = Block.query.get_or_404(block_id)
    if block.type != "quiz":
        flash("Этот блок не является тестом", "warning")
        return redirect(url_for("admin.course_edit", course_id=block.module.course_id))
    questions = QuizQuestion.query.filter_by(block_id=block.id).all()
    return render_template("admin/quiz_questions.html", block=block, questions=questions)


@admin_bp.route("/quizzes/<int:block_id>/questions/new", methods=["GET", "POST"])
@login_required
@admin_required
def quiz_question_new(block_id):
    block = Block.query.get_or_404(block_id)
    if block.type != "quiz":
        abort(404)
    form = QuizQuestionForm()
    if form.validate_on_submit():
        q = QuizQuestion(block_id=block.id, text=form.text.data)
        db.session.add(q)
        db.session.commit()
        flash("Вопрос добавлен", "success")
        return redirect(url_for("admin.quiz_edit", block_id=block.id))
    return render_template("admin/quiz_question_form.html", form=form, block=block, title="Новый вопрос")


@admin_bp.route("/quizzes/questions/<int:question_id>/edit", methods=["GET", "POST"])
@login_required
@admin_required
def quiz_question_edit(question_id):
    q = QuizQuestion.query.get_or_404(question_id)
    form = QuizQuestionForm(obj=q)
    if form.validate_on_submit():
        q.text = form.text.data
        db.session.commit()
        flash("Вопрос обновлён", "success")
        return redirect(url_for("admin.quiz_edit", block_id=q.block_id))
    return render_template("admin/quiz_question_form.html", form=form, block=q.block, title="Редактирование вопроса")


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


@admin_bp.route("/quizzes/questions/<int:question_id>/options/new", methods=["GET", "POST"])
@login_required
@admin_required
def quiz_option_new(question_id):
    question = QuizQuestion.query.get_or_404(question_id)
    form = QuizOptionForm()
    if form.validate_on_submit():
        opt = QuizOption(
            question_id=question.id,
            text=form.text.data,
            is_correct=(form.is_correct.data == "yes"),
        )
        db.session.add(opt)
        try:
            db.session.commit()
        except ValueError as e:
            # ловим ошибку из ORM-хука _ensure_single_correct
            db.session.rollback()
            flash(str(e) or "У этого вопроса уже есть правильный вариант ответа.", "danger")
        except IntegrityError:
            db.session.rollback()
            flash("У этого вопроса уже есть правильный вариант ответа.", "danger")
        else:
            flash("Вариант добавлен", "success")
            return redirect(url_for("admin.quiz_edit", block_id=question.block_id))

    return render_template(
        "admin/quiz_question_form.html",
        form=form,
        block=question.block,
        title=f"Новый вариант для вопроса #{question.id}",
    )


@admin_bp.route("/quizzes/options/<int:option_id>/edit", methods=["GET", "POST"])
@login_required
@admin_required
def quiz_option_edit(option_id):
    opt = QuizOption.query.get_or_404(option_id)
    form = QuizOptionForm(
        text=opt.text,
        is_correct="yes" if opt.is_correct else "no",
    )
    if form.validate_on_submit():
        opt.text = form.text.data
        opt.is_correct = (form.is_correct.data == "yes")
        try:
            db.session.commit()
        except ValueError as e:
            db.session.rollback()
            flash(str(e) or "У этого вопроса уже есть правильный вариант ответа.", "danger")
        except IntegrityError:
            db.session.rollback()
            flash("У этого вопроса уже есть правильный вариант ответа.", "danger")
        else:
            flash("Вариант обновлён", "success")
            return redirect(url_for("admin.quiz_edit", block_id=opt.question.block_id))

    return render_template(
        "admin/quiz_question_form.html",
        form=form,
        block=opt.question.block,
        title=f"Редактирование варианта для вопроса #{opt.question_id}",
    )


@admin_bp.route("/quizzes/options/<int:option_id>/delete", methods=["POST"])
@login_required
@admin_required
def quiz_option_delete(option_id):
    opt = QuizOption.query.get_or_404(option_id)
    block_id = opt.question.block_id
    db.session.delete(opt)
    db.session.commit()
    flash("Вариант удалён", "info")
    return redirect(url_for("admin.quiz_edit", block_id=block_id))
