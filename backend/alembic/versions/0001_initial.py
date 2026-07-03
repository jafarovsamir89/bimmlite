"""initial schema

Revision ID: 0001_initial
Revises:
Create Date: 2026-07-04 00:00:00.000000
"""

from __future__ import annotations

from alembic import op
import sqlalchemy as sa

revision = "0001_initial"
down_revision = None
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "logs",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("ts", sa.DateTime(timezone=True), nullable=False),
        sa.Column("level", sa.String(length=16), nullable=False),
        sa.Column("module", sa.String(length=80), nullable=False),
        sa.Column("event", sa.String(length=120), nullable=False),
        sa.Column("trace_id", sa.String(length=64), nullable=False),
        sa.Column("session_id", sa.String(length=64), nullable=False),
        sa.Column("user_id", sa.String(length=64), nullable=False, server_default=sa.text("'anonymous'")),
        sa.Column("vin", sa.String(length=32), nullable=False, server_default=sa.text("''")),
        sa.Column("ecu", sa.String(length=64), nullable=False, server_default=sa.text("''")),
        sa.Column("duration_ms", sa.Integer(), nullable=True),
        sa.Column("payload_hex", sa.Text(), nullable=False, server_default=sa.text("''")),
        sa.Column("result", sa.Text(), nullable=False, server_default=sa.text("''")),
        sa.Column("error", sa.Text(), nullable=False, server_default=sa.text("''")),
        sa.Column("message", sa.Text(), nullable=False, server_default=sa.text("''")),
    )
    op.create_index("ix_logs_trace_id", "logs", ["trace_id"])
    op.create_index("ix_logs_session_id", "logs", ["session_id"])

    op.create_table(
        "audit_log",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("ts", sa.DateTime(timezone=True), nullable=False),
        sa.Column("actor_id", sa.String(length=64), nullable=False, server_default=sa.text("'anonymous'")),
        sa.Column("trace_id", sa.String(length=64), nullable=False),
        sa.Column("session_id", sa.String(length=64), nullable=False),
        sa.Column("action", sa.String(length=120), nullable=False),
        sa.Column("target", sa.String(length=120), nullable=False, server_default=sa.text("''")),
        sa.Column("details", sa.Text(), nullable=False, server_default=sa.text("''")),
    )
    op.create_index("ix_audit_log_trace_id", "audit_log", ["trace_id"])
    op.create_index("ix_audit_log_session_id", "audit_log", ["session_id"])


def downgrade() -> None:
    op.drop_index("ix_audit_log_session_id", table_name="audit_log")
    op.drop_index("ix_audit_log_trace_id", table_name="audit_log")
    op.drop_table("audit_log")
    op.drop_index("ix_logs_session_id", table_name="logs")
    op.drop_index("ix_logs_trace_id", table_name="logs")
    op.drop_table("logs")
