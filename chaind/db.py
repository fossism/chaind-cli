import os
from pathlib import Path
from sqlmodel import SQLModel, create_engine, Session
from .models import Task  # Ensures the model is registered

CONFIG_DIR = Path.home() / ".config" / "chaind"
CONFIG_DIR.mkdir(parents=True, exist_ok=True)
DB_FILE = CONFIG_DIR / "chaind.db"
sqlite_url = f"sqlite:///{DB_FILE}"

engine = create_engine(sqlite_url, echo=False)

def create_db_and_tables():
    SQLModel.metadata.create_all(engine)

def get_session():
    return Session(engine)
