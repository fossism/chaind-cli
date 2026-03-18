from typer.testing import CliRunner
import os
import json
from pathlib import Path

from chaind.main import app
from chaind.db import CONFIG_DIR

runner = CliRunner()

def test_app_help():
    result = runner.invoke(app, ["--help"])
    assert result.exit_code == 0
    assert "ChainD - Developer Workflow Bridge" in result.stdout

def test_app_ls_empty():
    result = runner.invoke(app, ["ls"])
    assert result.exit_code == 0
    assert "No tasks found" in result.stdout

def test_app_auth():
    result = runner.invoke(app, ["auth"], input="testtest\ntesttoken\n")
    assert result.exit_code == 0
    assert "Successfully authenticated" in result.stdout

    config_file = CONFIG_DIR / "config.json"
    assert config_file.exists()
    
    with open(config_file, "r") as f:
        data = json.load(f)
        assert data["username"] == "testtest"
