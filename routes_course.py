# routes_course.py
import os
from datetime import datetime
from flask import Blueprint, render_template, request, redirect, url_for, flash, current_app
from flask_login import login_required, current_user
from werkzeug.utils import secure_filename

from models import (
    db, Course, Block, Submission,
    QuizQuestion, QuizOption, QuizAttempt
)

course_bp = Blueprint("course", __name__)

@course_bp.route("/courses")
@login_required
def list_courses():
    courses = Course.query.order_by(Course.created_at.desc()).all()
    return render_template("courses.html", courses=courses)

@course_bp.route("/course/<int:course_id>")
@login_required
def course_view(course_id):
    course = Course.query.get_or_404(course_id)
    return render_template("course_player.html", course=course)

# ---------- SUBMIT ASSIGNMENT (с гейтом по тесту) ----------
@course_bp.post("/course/blocks/<int:block_id>/submit")
@login_required
def submit_assignment(block_id):
    block = Block.query.get_or_404(block_id)
    if block.type != "assignment":
        flash("Это не задание", "danger")
        return redirect(url_for("course.course_view", course_id=block.module.course_id))

    # ГЕЙТ: если есть тест(ы) с require_pass=True в модуле — требуем прохождение
    module_quizzes = [blk for blk in block.module.blocks if blk.type == "quiz" and blk.payload.get("require_pass", True)]
    if module_quizzes:
        passed_any = (
            db.session.query(QuizAttempt)
            .filter(
                QuizAttempt.user_id == current_user.id,
                QuizAttempt.passed == True,
                QuizAttempt.block_id.in_([q.id for q in module_quizzes])
            )
            .first() is not None
        )
        if not passed_any:
            flash("Сначала пройдите тест по модулю, затем можно отправить задание.", "warning")
            return redirect(url_for("course.course_view", course_id=block.module.course_id) + f"#block-{block.id}")

    file = request.files.get("solution")
    if not file:
        flash("Файл не выбран", "warning")
        return redirect(url_for("course.course_view", course_id=block.module.course_id) + f"#block-{block.id}")

    uploads_dir = os.path.join(current_app.static_folder, "uploads", "submissions")
    os.makedirs(uploads_dir, exist_ok=True)

    safe = secure_filename(file.filename)
    name = f"{current_user.id}_{datetime.utcnow().strftime('%Y%m%d%H%M%S')}_{safe}"
    stored_rel = os.path.join("uploads", "submissions", name)
    file.save(os.path.join(current_app.static_folder, stored_rel))

    sub = Submission(
        block_id=block.id,
        user_id=current_user.id,
        original_name=file.filename,
        stored_path=stored_rel,
        size_bytes=os.path.getsize(os.path.join(current_app.static_folder, stored_rel)),
        status="submitted",
    )
    db.session.add(sub)
    db.session.commit()

    flash("Решение отправлено", "success")
    return redirect(url_for("course.course_view", course_id=block.module.course_id) + f"#block-{block.id}")

# ---------- SUBMIT QUIZ ----------
@course_bp.post("/course/quiz/<int:block_id>/submit")
@login_required
def submit_quiz(block_id: int):
    b = Block.query.get_or_404(block_id)
    if b.type != "quiz":
        flash("Это не тест", "danger")
        return redirect(url_for("course.course_view", course_id=b.module.course_id))

    answers = {}
    correct = 0
    total = len(b.quiz_questions)

    for q in b.quiz_questions:
        chosen = request.form.get(f"q-{q.id}")
        if not chosen:
            continue
        answers[str(q.id)] = int(chosen)
        opt = QuizOption.query.get(int(chosen))
        if opt and opt.is_correct:
            correct += 1

    score = int(round((correct / max(total, 1)) * 100))
    pass_score = int(b.payload.get("pass_score", 70))
    passed = score >= pass_score

    attempt = QuizAttempt(user_id=current_user.id, block_id=b.id, score=score, passed=passed, details=answers)
    db.session.add(attempt)
    db.session.commit()

    flash(f"Результат теста: {score}% ({'пройден' if passed else 'не пройден'})",
          "success" if passed else "warning")
    return redirect(url_for("course.course_view", course_id=b.module.course_id) + f"#block-{b.id}")
