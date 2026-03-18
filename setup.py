from setuptools import setup, find_packages

setup(
    name="chaind",
    version="0.1.0",
    description="A Developer Workflow Bridge to sync tasks, issues, and PRs into your local environment.",
    author="Rohit",
    packages=find_packages(),
    install_requires=[
        "typer>=0.9.0",
        "rich>=13.0.0",
        "requests>=2.31.0",
        "sqlmodel>=0.0.14",
    ],
    entry_points={
        "console_scripts": [
            "chaind=chaind.main:app",
        ],
    },
)
