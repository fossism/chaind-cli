import typer
import subprocess
import webbrowser
from rich.console import Console
from rich.table import Table
from rich import print as rprint
from sqlmodel import select

from .db import create_db_and_tables, get_session
from .github import save_config, fetch_assigned_tasks
from .models import Task

app = typer.Typer(help="ChainD - Developer Workflow Bridge")
console = Console()

@app.callback(invoke_without_command=True)
def callback(ctx: typer.Context):
    """
    ChainD initializing Database.
    """
    create_db_and_tables()
    if ctx.invoked_subcommand is None:
        rprint("[bold blue]Welcome to ChainD! Run `chaind --help` for commands.[/bold blue]")

@app.command()
def auth():
    """Authenticate with GitHub using a Personal Access Token."""
    rprint("[bold green]GitHub Authentication[/bold green]")
    username = typer.prompt("GitHub Username")
    token = typer.prompt("GitHub Personal Access Token", hide_input=True)
    save_config(token, username)
    rprint("[bold green]Successfully authenticated & cached locally![/bold green]")

@app.command()
def sync():
    """Sync your assigned issues and PRs from GitHub."""
    with console.status("[bold cyan]Fetching tasks from GitHub...[/bold cyan]") as status:
        try:
            tasks = fetch_assigned_tasks()
        except Exception as e:
            rprint(f"[bold red]Error:[/bold red] {e}")
            raise typer.Exit(1)
        
        with get_session() as session:
            synced_count = 0
            for t in tasks:
                statement = select(Task).where(Task.github_id == t.github_id)
                existing = session.exec(statement).first()
                if existing:
                    existing.title = t.title
                    existing.state = t.state
                    existing.updated_at = t.updated_at
                    # maintain local status unless closed remote
                    if existing.state == "closed":
                        existing.local_status = "done"
                    session.add(existing)
                else:
                    session.add(t)
                synced_count += 1
            session.commit()
    rprint(f"[bold green]Successfully synced {synced_count} tasks![/bold green]")

@app.command()
def ls(status: str = typer.Option("all", help="Filter by status: todo, in-progress, done, all")):
    """List your synced tasks in a beautiful table."""
    with get_session() as session:
        statement = select(Task)
        if status != "all":
            statement = statement.where(Task.local_status == status)
        
        # also ignore closed tasks by default unless explicitly asked? Let's show all locally tracked for now
        results = session.exec(statement).all()

        if not results:
            rprint("[yellow]No tasks found. Try running `chaind sync` first![/yellow]")
            return

        table = Table(title="Your Developer Tasks")
        table.add_column("ID", justify="right", style="cyan", no_wrap=True)
        table.add_column("Type", style="magenta")
        table.add_column("Repo", style="blue")
        table.add_column("Title", style="white")
        table.add_column("Status", style="green")

        for t in results:
            # Determine color based on status
            status_color = "[red]"
            if t.local_status == "todo":
                status_color = "[yellow]"
            elif t.local_status == "in-progress":
                status_color = "[blue]"
            elif t.local_status == "done":
                status_color = "[green]"

            type_emoji = "🐛" if t.type == "issue" else "🔗"
            
            table.add_row(
                str(t.number),
                f"{type_emoji} {t.type}",
                t.repo,
                t.title[:50] + ("..." if len(t.title) > 50 else ""),
                f"{status_color}{t.local_status}[/]"
            )

        console.print(table)

@app.command()
def start(repo: str, number: int):
    """Start working on a task (sets status to in-progress and checks out branch)."""
    with get_session() as session:
        statement = select(Task).where(Task.repo == repo).where(Task.number == number)
        task = session.exec(statement).first()
        if not task:
            rprint("[bold red]Task not found in local DB. Try `chaind sync`![/bold red]")
            raise typer.Exit(1)
            
        task.local_status = "in-progress"
        session.add(task)
        session.commit()
        
        # Attempt to branch off currently
        branch_name = f"issue-{number}"
        try:
            subprocess.run(["git", "checkout", "-b", branch_name], check=True)
            rprint(f"[bold green]Checked out new branch: {branch_name}[/bold green]")
        except subprocess.CalledProcessError:
            rprint(f"[yellow]Could not create git branch automatically (are you in a git repo?).[/yellow]")
        
        rprint(f"[bold blue]Marked `{repo}#{number}` as in-progress![/bold blue]")

@app.command()
def done(repo: str, number: int):
    """Mark a task as done locally."""
    with get_session() as session:
        statement = select(Task).where(Task.repo == repo).where(Task.number == number)
        task = session.exec(statement).first()
        if not task:
            rprint("[bold red]Task not found.[/bold red]")
            raise typer.Exit(1)
            
        task.local_status = "done"
        session.add(task)
        session.commit()
        
        rprint(f"[bold green]Awesome! `{repo}#{number}` marked as done.[/bold green]")
        rprint(f"To close it out, use this commit format:\n  [bold cyan]git commit -m \"Fixes #{number}: {task.title}\"[/bold cyan]")

@app.command()
def open(repo: str, number: int):
    """Open the task in your browser."""
    with get_session() as session:
        statement = select(Task).where(Task.repo == repo).where(Task.number == number)
        task = session.exec(statement).first()
        if not task:
            rprint("[bold red]Task not found.[/bold red]")
            raise typer.Exit(1)
            
        webbrowser.open(task.html_url)
        rprint(f"[bold green]Opened {task.html_url}[/bold green]")

if __name__ == "__main__":
    app()
