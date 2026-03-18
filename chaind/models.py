from typing import Optional
from sqlmodel import Field, SQLModel
from datetime import datetime

class Task(SQLModel, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    github_id: int = Field(unique=True, index=True)
    repo: str
    number: int
    title: str
    body: Optional[str] = None
    state: str
    type: str  # "issue" or "pr"
    html_url: str
    created_at: datetime
    updated_at: datetime
    local_status: str = Field(default="todo")  # todo, in-progress, done
