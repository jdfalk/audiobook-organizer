#!/usr/bin/env python3
# file: scripts/copilot-firewall/copilot_firewall/main.py
# version: 1.0.0
# guid: 9h0i1j2k-4c5d-6e7f-8g9h-0i1j2k3l4m5n

"""Main module for GitHub Copilot Firewall Allowlist Manager."""

import argparse
import json
import subprocess
import sys
from typing import Any, Dict, List

try:
    import inquirer
    from rich import box
    from rich.console import Console
    from rich.table import Table
except ImportError as e:
    console = Console()
    console.print(f"[red]Required dependency missing: {e}[/red]")
    console.print("[red]Please install with: pip install inquirer rich[/red]")
    sys.exit(1)

# The value for the variable
ALLOW_LIST = (
    "developer.mozilla.org,docs.github.com,docs.gitlab.com,docs.microsoft.com,"
    "learn.microsoft.com,docs.aws.amazon.com,cloud.google.com,docs.oracle.com,"
    "docs.docker.com,kubernetes.io,docs.ansible.com,docs.python.org,golang.org,"
    "pkg.go.dev,godoc.org,docs.npmjs.com,doc.rust-lang.org,cppreference.com,"
    "npmjs.com,pypi.org,rubygems.org,crates.io,nuget.org,packagist.org,"
    "api.github.com,developers.google.com,platform.openai.com,developer.apple.com,"
    "developer.amazon.com,docs.aws.amazon.com,azure.microsoft.com,cloud.google.com,"
    "developers.hubspot.com,developers.google.com,api.openai.com,platform.openai.com,"
    "gitlab.com,hub.docker.com,dockerhub.com,travis-ci.org,circleci.com,jenkins.io,"
    "codepen.io,jsfiddle.net,codesandbox.io,aws.amazon.com,azure.microsoft.com,"
    "digitalocean.com,heroku.com,firebase.google.com,cloudflare.com,ietf.org,w3.org,"
    "ecma-international.org,json.org,yaml.org,spec.openapis.org,semver.org,"
    "css-tricks.com,baeldung.com,geeksforgeeks.org,tutorialspoint.com,w3schools.com,"
    "freecodecamp.org,javatpoint.com,dev.to,medium.com,hackernoon.com,reactjs.org,"
    "vuejs.org,angular.io,nodejs.org,tensorflow.org,pytorch.org,fastapi.tiangolo.com,"
    "flask.palletsprojects.com,expressjs.com,laravel.com,symfony.com,rubyonrails.org,"
    "swift.org,dart.dev,flutter.dev,boost.org,numpy.org,pandas.pydata.org,"
    "scikit-learn.org,matplotlib.org,redis.io,mongodb.com,postgresql.org,mysql.com,"
    "sqlite.org,graphql.org,swagger.io,webpack.js.org,babeljs.io,typescriptlang.org,"
    "eslint.org,prettier.io,gnu.org,jetbrains.com,visualstudio.com,code.visualstudio.com,"
    "git-scm.com,cmake.org,docker.com,podman.io,kubernetes.io,istio.io,prometheus.io,"
    "grafana.com,elasticsearch.co,nginx.org,apache.org,grpc.io"
)

# Constants
MAX_DESCRIPTION_LENGTH = 60

console = Console()


class GitHubManager:
    """Handles GitHub CLI operations."""

    def __init__(self, org: str = "jdfalk") -> None:
        """
        Initialize GitHub manager.

        Args:
            org: GitHub organization or username
        """
        self.org = org

    def check_prerequisites(self) -> bool:
        """
        Check if GitHub CLI is installed and authenticated.

        Returns:
            bool: True if all prerequisites are met, False otherwise
        """
        # Check if gh is installed
        try:
            subprocess.run(
                ["gh", "--version"],
                check=True,
                capture_output=True,
            )
        except (subprocess.SubprocessError, FileNotFoundError):
            console.print(
                "[red]GitHub CLI (gh) is not installed. Please install it first.[/red]"
            )
            console.print("Visit: https://cli.github.com/manual/installation")
            return False

        # Check if user is authenticated
        try:
            subprocess.run(
                ["gh", "auth", "status"],
                check=True,
                capture_output=True,
            )
        except subprocess.SubprocessError:
            console.print(
                "[red]You are not logged in to GitHub CLI. Please run 'gh auth login' first.[/red]"
            )
            return False

        return True

    def get_repositories(self, limit: int = 100) -> List[Dict[str, Any]]:
        """
        Fetch repositories from the specified GitHub organization or user.

        Args:
            limit: Maximum number of repositories to fetch

        Returns:
            List of repository information dictionaries
        """
        try:
            result = subprocess.run(
                [
                    "gh",
                    "repo",
                    "list",
                    self.org,
                    "--limit",
                    str(limit),
                    "--json",
                    "name,description,visibility,isPrivate,updatedAt",
                ],
                check=True,
                stdout=subprocess.PIPE,
                text=True,
            )
            repos = json.loads(result.stdout)

            # Sort by name for consistent display
            return sorted(repos, key=lambda x: x["name"].lower())
        except subprocess.SubprocessError as e:
            console.print(f"[red]Error fetching repositories: {e}[/red]")
            return []

    def set_variable(self, repo_name: str) -> bool:
        """
        Set the COPILOT_AGENT_FIREWALL_ALLOW_LIST_ADDITIONS variable for a repository.

        Args:
            repo_name: Name of the repository

        Returns:
            bool: True if successful, False otherwise
        """
        try:
            subprocess.run(
                [
                    "gh",
                    "variable",
                    "set",
                    "COPILOT_AGENT_FIREWALL_ALLOW_LIST_ADDITIONS",
                    "-b",
                    ALLOW_LIST,
                    "-R",
                    f"{self.org}/{repo_name}",
                ],
                check=True,
                capture_output=True,
            )
        except subprocess.SubprocessError as e:
            console.print(f"[red]Error setting variable for {repo_name}: {e}[/red]")
            return False
        else:
            return True


def display_repositories(repos: List[Dict[str, Any]]) -> None:
    """
    Display repositories in a formatted table.

    Args:
        repos: List of repository information dictionaries
    """
    table = Table(title="Available Repositories", box=box.ROUNDED)
    table.add_column("Repository", style="cyan", no_wrap=True)
    table.add_column("Visibility", style="magenta")
    table.add_column("Description", style="green", max_width=50)
    table.add_column("Last Updated", style="yellow")

    for repo in repos:
        visibility = "🔒 Private" if repo.get("isPrivate", False) else "🌍 Public"
        description = repo.get("description", "No description") or "No description"
        updated = repo.get("updatedAt", "Unknown")[:10]  # Just the date part

        table.add_row(repo["name"], visibility, description, updated)

    console.print(table)


def filter_repositories(
    repos: List[Dict[str, Any]], filter_term: str = ""
) -> List[Dict[str, Any]]:
    """
    Filter repositories by name or description.

    Args:
        repos: List of repository information dictionaries
        filter_term: Term to filter by

    Returns:
        Filtered list of repositories
    """
    if not filter_term:
        return repos

    filter_term = filter_term.lower()
    return [
        repo
        for repo in repos
        if (
            filter_term in repo["name"].lower()
            or (repo.get("description") and filter_term in repo["description"].lower())
        )
    ]


def _get_user_action() -> str:
    """Get the user's desired action."""
    options = [
        "🎯 Select specific repositories",
        "🌟 Select all repositories",
        "🔍 Filter and select repositories",
        "❌ Cancel operation",
    ]

    question = [
        inquirer.List(
            "action",
            message="What would you like to do?",
            choices=options,
        ),
    ]

    answer = inquirer.prompt(question)
    return answer["action"] if answer else "❌ Cancel operation"


def _filter_repositories_interactively(
    repos: List[Dict[str, Any]],
) -> List[Dict[str, Any]]:
    """Filter repositories based on user input."""
    filter_term = input("Enter filter term (name or description): ").strip()
    if not filter_term:
        return repos

    filtered_repos = filter_repositories(repos, filter_term)
    console.print(
        f"\n[green]Found {len(filtered_repos)} repositories matching '{filter_term}'[/green]"
    )

    if filtered_repos:
        display_repositories(filtered_repos)
        return filtered_repos

    console.print("[yellow]No repositories match the filter.[/yellow]")
    return []


def _select_repositories_from_choices(repos: List[Dict[str, Any]]) -> List[str]:
    """Present repository choices and get user selection."""
    choices = [
        {
            "name": f"{repo['name']} ({'🔒' if repo.get('isPrivate') else '🌍'}) - {repo.get('description', 'No description')[:MAX_DESCRIPTION_LENGTH]}{'...' if repo.get('description', '') and len(repo.get('description', '')) > MAX_DESCRIPTION_LENGTH else ''}",
            "value": repo["name"],
            "checked": False,
        }
        for repo in repos
    ]

    questions = [
        inquirer.Checkbox(
            "selected_repos",
            message="Select repositories to set COPILOT_AGENT_FIREWALL_ALLOW_LIST_ADDITIONS variable",
            choices=choices,
        ),
    ]

    answers = inquirer.prompt(questions)
    if not answers:
        return []

    selected = answers["selected_repos"]
    if not selected:
        return []

    # Handle both string values and dictionary values
    repo_names = []
    for item in selected:
        if isinstance(item, dict):
            repo_names.append(item.get("value", item.get("name", str(item))))
        else:
            repo_names.append(str(item))

    return repo_names


def select_repositories(repos: List[Dict[str, Any]]) -> List[str]:
    """
    Present an interactive selection interface to choose repositories.

    Args:
        repos: List of repository information dictionaries

    Returns:
        List of selected repository names
    """
    if not repos:
        console.print(
            "[yellow]No repositories found or you don't have access to the namespace.[/yellow]"
        )
        return []

    action = _get_user_action()

    if action == "❌ Cancel operation":
        return []

    if action == "🌟 Select all repositories":
        return [repo["name"] for repo in repos]

    if action == "🔍 Filter and select repositories":
        repos = _filter_repositories_interactively(repos)
        if not repos:
            return []

    return _select_repositories_from_choices(repos)


def _setup_argument_parser() -> argparse.ArgumentParser:
    """Set up the command line argument parser."""
    parser = argparse.ArgumentParser(
        description="Set COPILOT_AGENT_FIREWALL_ALLOW_LIST_ADDITIONS for GitHub repositories"
    )
    parser.add_argument(
        "--org",
        default="jdfalk",
        help="GitHub organization or username (default: jdfalk)",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=100,
        help="Maximum number of repositories to fetch (default: 100)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be done without actually setting variables",
    )
    parser.add_argument(
        "--list-only",
        action="store_true",
        help="Only list repositories and exit",
    )
    return parser


def _handle_dry_run(selected_repos: List[str], org: str) -> None:
    """Handle dry run mode."""
    console.print("\n[yellow]DRY RUN: No variables will be set.[/yellow]")
    console.print(
        "The following repositories would have their COPILOT_AGENT_FIREWALL_ALLOW_LIST_ADDITIONS variable set:"
    )
    for repo in selected_repos:
        console.print(f"  • [cyan]{org}/{repo}[/cyan]")


def _set_variables(selected_repos: List[str], gh_manager: GitHubManager) -> None:
    """Set variables for selected repositories."""
    console.print("\n[yellow]Setting variables...[/yellow]")
    success_count = 0
    failed_repos = []

    for repo in selected_repos:
        console.print(f"Setting variable for [cyan]{repo}[/cyan]...", end=" ")

        if gh_manager.set_variable(repo):
            console.print("[green]✅ Success[/green]")
            success_count += 1
        else:
            console.print("[red]❌ Failed[/red]")
            failed_repos.append(repo)

    # Summary
    console.print("\n[bold]Operation completed![/bold]")
    console.print(
        f"[green]✅ Successfully set variable for {success_count} repositories[/green]"
    )

    if failed_repos:
        console.print(
            f"[red]❌ Failed to set variable for {len(failed_repos)} repositories:[/red]"
        )
        for repo in failed_repos:
            console.print(f"  • {repo}")


def main() -> None:
    """Main entry point for the copilot firewall manager."""
    parser = _setup_argument_parser()
    args = parser.parse_args()

    console.print("[bold blue]GitHub Copilot Firewall Allowlist Manager[/bold blue]")
    console.print(f"Organization: [cyan]{args.org}[/cyan]\n")

    # Initialize GitHub manager
    gh_manager = GitHubManager(args.org)

    # Check prerequisites
    if not gh_manager.check_prerequisites():
        sys.exit(1)

    # Get repositories
    console.print("[yellow]Fetching repositories...[/yellow]")
    repos = gh_manager.get_repositories(args.limit)

    if not repos:
        console.print("[red]No repositories found.[/red]")
        sys.exit(1)

    # Display repositories
    display_repositories(repos)

    if args.list_only:
        console.print(f"\n[green]Found {len(repos)} repositories.[/green]")
        return

    # Select repositories
    selected_repos = select_repositories(repos)

    if not selected_repos:
        console.print("[yellow]No repositories selected. Exiting.[/yellow]")
        return

    console.print(f"\n[green]Selected {len(selected_repos)} repositories:[/green]")
    for repo in selected_repos:
        console.print(f"  • {repo}")

    if args.dry_run:
        _handle_dry_run(selected_repos, args.org)
        return

    # Confirm before proceeding
    if not inquirer.confirm(
        "Proceed with setting the variable for selected repositories?"
    ):
        console.print("[yellow]Operation cancelled.[/yellow]")
        return

    # Set variables
    _set_variables(selected_repos, gh_manager)


if __name__ == "__main__":
    main()
