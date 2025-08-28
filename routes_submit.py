# routes_submit.py
import uuid
from pathlib import Path
from werkzeug.utils import secure_filename
from flask import Blueprint, current_app, request, redirect, url_for, flash, abort
from flask_login import login_required, current_user
from models import db, Block, Submission

submit_bp = Blueprint("submit", __name__)

ALLOWED_EXT = {"pdf","png","jpg","jpeg","txt","py","zip","doc","docx","pptx","xlsx","csv","ipynb"}

def _allowed(filename: str) -> bool:
    return "." in filename and filename.rsplit(".", 1)[1].lower() in ALLOWED_EXT

@submit_bp.post("/submit/<int:block_id>")
@login_required
def upload(block_id: int):
    b = Block.query.get_or_404(block_id)
    if b.type != "assignment":
        abort(400, "Submitting is allowed only for assignment blocks")

    file = request.files.get("solution")
    if not file or file.filename == "":
        flash("Файл не выбран", "warning")
        return redirect(url_for("course.course_view", course_id=b.module.course_id) + f"#block-{b.id}")

    if not _allowed(file.filename):
        flash("Недопустимое расширение файла", "danger")
        return redirect(url_for("course.course_view", course_id=b.module.course_id) + f"#block-{b.id}")

    # путь сохранения
    rel_dir = current_app.config["SUBMISSIONS_REL_PATH"]
    base = Path(current_app.root_path) / "static" / rel_dir
    base.mkdir(parents=True, exist_ok=True)

    safe_name = secure_filename(file.filename)
    stored_name = f"{uuid.uuid4().hex}_{safe_name}"
    abs_path = base / stored_name

    file.save(abs_path)

    sub = Submission(
        user_id=current_user.id,
        block_id=b.id,
        original_name=safe_name,
        stored_path=f"{rel_dir}/{stored_name}",
        mimetype=file.mimetype,
        size_bytes=abs_path.stat().st_size,
        comment="",
        status="submitted",
    )
    db.session.add(sub)
    db.session.commit()

    flash("Решение отправлено", "success")
    return redirect(url_for("course.course_view", course_id=b.module.course_id) + f"#block-{b.id}")
