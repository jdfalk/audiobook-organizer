#!/usr/bin/env python3
# file: scripts/mark_old_notifications_done.py
# version: 1.1.0
# guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

"""
GitHub Notifications Cleanup Script

This script marks all GitHub notifications older than 24 hours as done.
It uses the GitHub API to fetch unread notifications and marks those older
than the specified threshold as done (completely dismissed).

Requirements:
- GitHub Personal Access Token with 'notifications' scope
- Set as GITHUB_TOKEN environment variable or in .env file

Usage:
    python scripts/mark_old_notifications_done.py [--hours HOURS] [--dry-run]

Arguments:
    --hours HOURS    Number of hours old notifications must be to mark as done (default: 24)
    --dry-run        Show what would be marked as done without actually doing it
"""

import argparse
import os
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timedelta, timezone
from typing import Dict, List, Optional, Tuple

import requests
from dotenv import load_dotenv


class GitHubNotificationCleaner:
    """
    Manages cleanup of old GitHub notifications using the GitHub API.

    This class provides functionality to fetch unread notifications and
    mark those older than a specified threshold as done (completely dismissed).
    """

    def __init__(self, token: str, max_workers: int = 10):
        """
        Initialize the notification cleaner with a GitHub token.

        Args:
            token: GitHub Personal Access Token with notifications scope
            max_workers: Maximum number of concurrent workers for parallel processing
        """
        self.token = token
        self.max_workers = max_workers
        self.session = requests.Session()
        self.session.headers.update(
            {
                "Authorization": f"token {token}",
                "Accept": "application/vnd.github.v3+json",
                "User-Agent": "GitHub-Notification-Cleaner/1.1",
            }
        )
        self.base_url = "https://api.github.com"
        self._rate_limit_lock = threading.Lock()
        self._last_request_time = 0
        self._min_request_interval = (
            0.1  # 100ms between requests to respect rate limits
        )

    def get_notifications(self, all_notifications: bool = False) -> List[Dict]:
        """
        Fetch notifications from GitHub API.

        Args:
            all_notifications: If True, fetch all notifications; if False, only unread

        Returns:
            List of notification dictionaries from the GitHub API

        Raises:
            requests.RequestException: If API request fails
        """
        url = f"{self.base_url}/notifications"
        params = {"all": str(all_notifications).lower(), "per_page": 100}

        notifications = []
        page = 1

        while True:
            params["page"] = page

            try:
                response = self.session.get(url, params=params)
                response.raise_for_status()

                page_notifications = response.json()
                if not page_notifications:
                    break

                notifications.extend(page_notifications)
                page += 1

                # GitHub API typically limits to 50 pages
                if page > 50:
                    break

            except requests.RequestException as e:
                print(f"Error fetching notifications: {e}", file=sys.stderr)
                raise

        return notifications

    def parse_notification_time(self, time_str: str) -> datetime:
        """
        Parse GitHub notification timestamp to datetime object.

        Args:
            time_str: ISO 8601 timestamp string from GitHub API

        Returns:
            datetime object in UTC timezone
        """
        # GitHub returns timestamps like "2023-12-25T10:30:00Z"
        return datetime.fromisoformat(time_str.replace("Z", "+00:00"))

    def is_notification_old(self, notification: Dict, hours_threshold: int) -> bool:
        """
        Check if a notification is older than the specified threshold.

        Args:
            notification: Notification dictionary from GitHub API
            hours_threshold: Number of hours old the notification must be

        Returns:
            True if notification is older than threshold, False otherwise
        """
        updated_at = self.parse_notification_time(notification["updated_at"])
        threshold_time = datetime.now(timezone.utc) - timedelta(hours=hours_threshold)
        return updated_at < threshold_time

    def _rate_limit_wait(self):
        """
        Implement rate limiting to respect GitHub API limits.
        Ensures minimum interval between requests.
        """
        with self._rate_limit_lock:
            current_time = time.time()
            time_since_last = current_time - self._last_request_time
            if time_since_last < self._min_request_interval:
                time.sleep(self._min_request_interval - time_since_last)
            self._last_request_time = time.time()

    def mark_notification_as_done(self, notification_id: str) -> Tuple[bool, str]:
        """
        Mark a specific notification as done (completely dismiss it).

        Args:
            notification_id: GitHub notification ID

        Returns:
            Tuple of (success: bool, error_message: str)
        """
        self._rate_limit_wait()
        url = f"{self.base_url}/notifications/threads/{notification_id}"

        try:
            # Use DELETE method to mark as done (completely dismiss)
            response = self.session.delete(url)
            response.raise_for_status()
            return True, ""
        except requests.RequestException as e:
            error_msg = f"Error marking notification {notification_id} as done: {e}"
            return False, error_msg

    def mark_all_as_done(self) -> bool:
        """
        Mark all notifications as done (completely dismiss them).
        Note: GitHub API doesn't support bulk delete, so this marks them as read.

        Returns:
            True if successful, False otherwise
        """
        url = f"{self.base_url}/notifications"

        try:
            response = self.session.put(url)
            response.raise_for_status()
            return True
        except requests.RequestException as e:
            print(f"Error marking all notifications as done: {e}", file=sys.stderr)
            return False

    def _process_notification_batch(
        self, notifications: List[Dict], dry_run: bool = False
    ) -> Tuple[int, int, List[str]]:
        """
        Process a batch of notifications in parallel.

        Args:
            notifications: List of notification dictionaries to process
            dry_run: If True, only simulate the actions

        Returns:
            Tuple of (marked_count, failed_count, error_messages)
        """
        if dry_run:
            for notification in notifications:
                subject = notification["subject"]["title"]
                repo = (
                    notification["repository"]["full_name"]
                    if notification["repository"]
                    else "Unknown"
                )
                updated_at = notification["updated_at"]
                print(
                    f"[DRY RUN] Would mark as done: {repo} - {subject} (updated: {updated_at})"
                )
            return len(notifications), 0, []

        marked_count = 0
        failed_count = 0
        error_messages = []

        with ThreadPoolExecutor(max_workers=self.max_workers) as executor:
            # Submit all tasks
            future_to_notification = {}
            for notification in notifications:
                future = executor.submit(
                    self.mark_notification_as_done, notification["id"]
                )
                future_to_notification[future] = notification

            # Process completed tasks
            for future in as_completed(future_to_notification):
                notification = future_to_notification[future]
                subject = notification["subject"]["title"]
                repo = (
                    notification["repository"]["full_name"]
                    if notification["repository"]
                    else "Unknown"
                )
                updated_at = notification["updated_at"]

                try:
                    success, error_msg = future.result()
                    if success:
                        print(
                            f"✓ Marked as done: {repo} - {subject} (updated: {updated_at})"
                        )
                        marked_count += 1
                    else:
                        print(f"✗ Failed: {repo} - {subject} (updated: {updated_at})")
                        failed_count += 1
                        if error_msg:
                            error_messages.append(error_msg)
                except Exception as e:
                    print(f"✗ Exception: {repo} - {subject} - {e}")
                    failed_count += 1
                    error_messages.append(str(e))

        return marked_count, failed_count, error_messages

    def cleanup_old_notifications(
        self, hours_threshold: int = 24, dry_run: bool = False, batch_size: int = 100
    ) -> Dict[str, int]:
        """
        Main method to clean up old notifications using parallel processing.

        Args:
            hours_threshold: Number of hours old notifications must be to mark as done
            dry_run: If True, only show what would be done without actually doing it
            batch_size: Number of notifications to process in each batch

        Returns:
            Dictionary with counts of processed, marked, and failed notifications
        """
        print("Fetching unread notifications...")

        try:
            notifications = self.get_notifications(all_notifications=False)
        except requests.RequestException:
            return {"processed": 0, "marked": 0, "failed": 0, "error": True}

        print(f"Found {len(notifications)} unread notifications")

        old_notifications = []
        for notification in notifications:
            if self.is_notification_old(notification, hours_threshold):
                old_notifications.append(notification)

        print(
            f"Found {len(old_notifications)} notifications older than {hours_threshold} hours"
        )

        if not old_notifications:
            print("No old notifications to process")
            return {"processed": 0, "marked": 0, "failed": 0}

        total_marked = 0
        total_failed = 0
        all_errors = []

        # Process notifications in batches
        total_batches = (len(old_notifications) + batch_size - 1) // batch_size
        print(
            f"Processing in {total_batches} batches of up to {batch_size} notifications each..."
        )
        print(f"Using up to {self.max_workers} concurrent workers per batch")

        for i in range(0, len(old_notifications), batch_size):
            batch_num = (i // batch_size) + 1
            batch = old_notifications[i : i + batch_size]

            print(
                f"\nProcessing batch {batch_num}/{total_batches} ({len(batch)} notifications)..."
            )

            batch_marked, batch_failed, batch_errors = self._process_notification_batch(
                batch, dry_run
            )

            total_marked += batch_marked
            total_failed += batch_failed
            all_errors.extend(batch_errors)

            print(
                f"Batch {batch_num} complete: {batch_marked} marked, {batch_failed} failed"
            )

        result = {
            "processed": len(old_notifications),
            "marked": total_marked,
            "failed": total_failed,
        }

        if dry_run:
            print(f"\n[DRY RUN] Would have marked {total_marked} notifications as done")
        else:
            print(f"\nMarked {total_marked} notifications as done")
            if total_failed > 0:
                print(f"Failed to mark {total_failed} notifications")
                if all_errors and len(all_errors) <= 10:  # Show first 10 errors
                    print("First few errors:")
                    for error in all_errors[:10]:
                        print(f"  - {error}")

        return result


def get_github_token() -> Optional[str]:
    """
    Get GitHub token from environment or .env file.

    Returns:
        GitHub token string if found, None otherwise
    """
    # Load .env file if present
    load_dotenv()

    # Try different environment variable names
    token = (
        os.getenv("GITHUB_TOKEN") or os.getenv("GH_TOKEN") or os.getenv("JF_CI_GH_PAT")
    )

    if not token:
        print(
            "Error: GitHub token not found. Please set GITHUB_TOKEN environment variable.",
            file=sys.stderr,
        )
        print(
            "The token needs 'notifications' scope to read and modify notifications.",
            file=sys.stderr,
        )
        return None

    return token


def main():
    """
    Main entry point for the script.
    """
    parser = argparse.ArgumentParser(
        description="Mark old GitHub notifications as done (completely dismissed)",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    # Mark notifications older than 24 hours as done
    python scripts/mark_old_notifications_done.py

    # Mark notifications older than 48 hours as done
    python scripts/mark_old_notifications_done.py --hours 48

    # Dry run to see what would be marked as done
    python scripts/mark_old_notifications_done.py --dry-run

    # Use more workers for faster processing
    python scripts/mark_old_notifications_done.py --workers 20

    # Process larger batches (be careful with API limits)
    python scripts/mark_old_notifications_done.py --batch-size 200 --workers 15

    # Mark notifications older than 12 hours (dry run)
    python scripts/mark_old_notifications_done.py --hours 12 --dry-run
        """,
    )

    parser.add_argument(
        "--hours",
        type=int,
        default=24,
        help="Number of hours old notifications must be to mark as done (default: 24)",
    )

    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be marked as done without actually doing it",
    )

    parser.add_argument(
        "--workers",
        type=int,
        default=10,
        help="Maximum number of concurrent workers for parallel processing (default: 10)",
    )

    parser.add_argument(
        "--batch-size",
        type=int,
        default=100,
        help="Number of notifications to process in each batch (default: 100)",
    )

    parser.add_argument("--verbose", action="store_true", help="Enable verbose output")

    args = parser.parse_args()

    # Validate arguments
    if args.hours < 1:
        print("Error: Hours must be a positive integer", file=sys.stderr)
        sys.exit(1)

    if args.workers < 1 or args.workers > 50:
        print("Error: Workers must be between 1 and 50", file=sys.stderr)
        sys.exit(1)

    if args.batch_size < 1 or args.batch_size > 1000:
        print("Error: Batch size must be between 1 and 1000", file=sys.stderr)
        sys.exit(1)

    # Get GitHub token
    token = get_github_token()
    if not token:
        sys.exit(1)

    # Initialize cleaner and run cleanup
    cleaner = GitHubNotificationCleaner(token, max_workers=args.workers)

    print("GitHub Notification Cleanup")
    print(f"Threshold: {args.hours} hours")
    print(f"Workers: {args.workers}")
    print(f"Batch size: {args.batch_size}")
    print(f"Mode: {'DRY RUN' if args.dry_run else 'LIVE'}")
    print("-" * 50)

    try:
        result = cleaner.cleanup_old_notifications(
            hours_threshold=args.hours, dry_run=args.dry_run, batch_size=args.batch_size
        )

        if result.get("error"):
            sys.exit(1)

        # Print summary
        print("-" * 50)
        print("Summary:")
        print(f"  Notifications processed: {result['processed']}")
        print(f"  Notifications marked as done: {result['marked']}")
        if result["failed"] > 0:
            print(f"  Failed to mark: {result['failed']}")

    except KeyboardInterrupt:
        print("\nOperation cancelled by user")
        sys.exit(1)
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
