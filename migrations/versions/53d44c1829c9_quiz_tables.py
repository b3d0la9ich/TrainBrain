"""quiz tables

Revision ID: 53d44c1829c9
Revises: 1d4b1c479147
Create Date: 2025-08-28 07:04:52.070620
"""
from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision = '53d44c1829c9'
down_revision = '1d4b1c479147'
branch_labels = None
depends_on = None


def upgrade():
    # --- таблицы ---
    op.create_table(
        'quiz_attempt',
        sa.Column('id', sa.Integer(), nullable=False),
        sa.Column('user_id', sa.Integer(), nullable=False),
        sa.Column('block_id', sa.Integer(), nullable=False),
        sa.Column('score', sa.Integer(), nullable=True),
        sa.Column('passed', sa.Boolean(), nullable=True),
        sa.Column('submitted_at', sa.DateTime(), nullable=True),
        sa.Column('details', sa.JSON(), nullable=True),
        sa.ForeignKeyConstraint(['block_id'], ['block.id']),
        sa.ForeignKeyConstraint(['user_id'], ['user.id']),
        sa.PrimaryKeyConstraint('id')
    )

    op.create_table(
        'quiz_question',
        sa.Column('id', sa.Integer(), nullable=False),
        sa.Column('block_id', sa.Integer(), nullable=False),
        sa.Column('text', sa.Text(), nullable=False),
        sa.Column('order', sa.Integer(), nullable=True),
        sa.ForeignKeyConstraint(['block_id'], ['block.id']),
        sa.PrimaryKeyConstraint('id')
    )

    op.create_table(
        'quiz_option',
        sa.Column('id', sa.Integer(), nullable=False),
        sa.Column('question_id', sa.Integer(), nullable=False),
        sa.Column('text', sa.Text(), nullable=False),
        sa.Column('is_correct', sa.Boolean(), nullable=True),
        sa.ForeignKeyConstraint(['question_id'], ['quiz_question.id']),
        sa.PrimaryKeyConstraint('id')
    )

    # --- частичный уникальный индекс: только один правильный вариант на вопрос ---
    op.create_index(
        'uq_quizoption_one_correct',
        'quiz_option',
        ['question_id'],
        unique=True,
        postgresql_where=sa.text('is_correct = TRUE')
    )


def downgrade():
    # сначала удаляем индекс, затем таблицы
    op.drop_index('uq_quizoption_one_correct', table_name='quiz_option')
    op.drop_table('quiz_option')
    op.drop_table('quiz_question')
    op.drop_table('quiz_attempt')
