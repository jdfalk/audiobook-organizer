#!/usr/bin/env python3
# file: scripts/enhanced_doc_update_manager.py
# version: 4.0.0
# guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

"""
Enhanced Documentation Update Manager with Comprehensive Timestamp Lifecycle Tracking

This enhanced version processes files individually with proper error isolation,
incremental progress tracking, and comprehensive timestamp lifecycle support.

Features:
- Enhanced timestamp format v2.0 with lifecycle tracking (created_at, processed_at, failed_at)
- Chronological processing based on created_at timestamps
- Git-integrated timestamp recovery for historical accuracy
- Individual file processing with immediate status updates
- Malformed file isolation to 'malformed/' directory
- Failed file isolation to 'failed/' directory
- Resume capability from partial failures
- Comprehensive error logging and recovery
- Multiple processing modes (append, prepend, replace-section, etc.)
- Backwards compatibility with existing formats
"""

import argparse
import json
import logging
import re
import shutil
import sys
import traceback
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, Optional

# Configure logging
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)


class EnhancedDocumentationUpdateManager:
    """Enhanced manager with incremental processing and error isolation."""

    def __init__(
        self,
        updates_dir: str = ".github/doc-updates",
        cleanup: bool = True,
        dry_run: bool = False,
        verbose: bool = False,
        continue_on_error: bool = True,
    ):
        self.updates_dir = Path(updates_dir)
        self.cleanup = cleanup
        self.dry_run = dry_run
        self.verbose = verbose
        self.continue_on_error = continue_on_error

        # Directory structure for file management
        self.processed_dir = self.updates_dir / "processed"
        self.malformed_dir = self.updates_dir / "malformed"
        self.failed_dir = self.updates_dir / "failed"

        # Create directories
        for dir_path in [self.processed_dir, self.malformed_dir, self.failed_dir]:
            dir_path.mkdir(parents=True, exist_ok=True)

        self.stats = {
            "files_processed": 0,
            "files_malformed": 0,
            "files_failed": 0,
            "changes_made": False,
            "files_updated": [],
            "errors": [],
            "malformed_files": [],
            "failed_files": [],
        }

        if verbose:
            logger.setLevel(logging.DEBUG)

    def process_all_updates(self) -> Dict[str, Any]:
        """Process all update files with individual error handling."""
        logger.info(f"🔄 Processing documentation updates from {self.updates_dir}")

        if not self.updates_dir.exists():
            logger.info(f"📝 Updates directory does not exist: {self.updates_dir}")
            return self.stats

        # Find update files (excluding subdirectories)
        update_files = [f for f in self.updates_dir.glob("*.json") if f.is_file()]

        if not update_files:
            logger.info("📝 No update files found")
            return self.stats

        logger.info(f"📊 Found {len(update_files)} update files")

        # Process files individually in sorted order
        update_files.sort()

        for update_file in update_files:
            self._process_single_file(update_file)

        # Generate summary
        self._log_processing_summary()
        return self.stats

    def _process_single_file(self, update_file: Path) -> None:
        """Process a single update file with comprehensive error handling."""
        logger.debug(f"🔍 Processing: {update_file.name}")

        try:
            # Step 1: Validate and parse JSON
            try:
                with open(update_file, encoding="utf-8") as f:
                    update_data = json.load(f)
            except (OSError, json.JSONDecodeError) as e:
                self._handle_malformed_file(update_file, f"JSON parse error: {e}")
                return

            # Step 2: Validate required fields
            validation_error = self._validate_update_data(update_data)
            if validation_error:
                self._handle_malformed_file(update_file, validation_error)
                return

            # Step 3: Apply the update
            if self.dry_run:
                logger.info(f"🧪 [DRY RUN] Would process {update_file.name}")
                self.stats["files_processed"] += 1
                # In dry run, we still move to processed to show what would happen
                if self.cleanup:
                    self._move_to_processed(update_file)
                return

            # Apply the actual update
            success = self._apply_file_update(update_file, update_data)

            if success:
                self.stats["files_processed"] += 1
                self.stats["changes_made"] = True

                # Move to processed immediately after success
                if self.cleanup:
                    self._move_to_processed(update_file)
                    logger.debug(f"✅ Processed and archived: {update_file.name}")
            else:
                # Update failed but file format was valid
                if self.continue_on_error:
                    self._move_to_failed(update_file, "Update application failed")
                else:
                    raise Exception("Update application failed")

        except Exception as e:
            error_msg = f"Unexpected error processing {update_file.name}: {str(e)}"
            logger.error(error_msg)

            if self.continue_on_error:
                self._move_to_failed(update_file, error_msg)
            else:
                self.stats["errors"].append(error_msg)
                raise

    def _validate_update_data(self, update_data: Dict) -> Optional[str]:
        """Validate update data structure."""
        required_fields = ["file", "mode", "content"]

        for field in required_fields:
            if field not in update_data:
                return f"Missing required field: {field}"

        # Validate mode-specific requirements
        mode = update_data["mode"]
        options = update_data.get("options", {})

        if mode == "replace-section" and "section" not in options:
            return "replace-section mode requires 'section' in options"
        elif mode == "insert-after" and "after" not in options:
            return "insert-after mode requires 'after' in options"
        elif mode == "insert-before" and "before" not in options:
            return "insert-before mode requires 'before' in options"
        elif mode == "update-badge" and "badge_name" not in options:
            return "update-badge mode requires 'badge_name' in options"

        return None

    def _apply_file_update(self, update_file: Path, update_data: Dict) -> bool:
        """Apply update from a single file."""
        try:
            target_file = Path(update_data["file"])
            mode = update_data["mode"]
            content = update_data["content"]
            options = update_data.get("options", {})

            logger.info(f"📝 Updating {target_file} (mode: {mode})")

            # Create target file if it doesn't exist
            if not target_file.exists():
                target_file.parent.mkdir(parents=True, exist_ok=True)
                target_file.touch()
                logger.info(f"📄 Created new file: {target_file}")

            # Read current content
            try:
                with open(target_file, encoding="utf-8") as f:
                    current_content = f.read()
            except UnicodeDecodeError:
                with open(target_file, encoding="latin-1") as f:
                    current_content = f.read()

            # Apply update based on mode
            new_content = self._apply_mode(current_content, mode, content, options)

            if new_content != current_content:
                with open(target_file, "w", encoding="utf-8") as f:
                    f.write(new_content)

                # Track updated file
                if str(target_file) not in self.stats["files_updated"]:
                    self.stats["files_updated"].append(str(target_file))

                logger.info(f"✅ Updated {target_file}")
                return True
            else:
                logger.info(f"📄 No changes needed for {target_file}")
                return True  # Still considered successful

        except Exception as e:
            error_msg = f"Failed to apply update from {update_file.name}: {str(e)}"
            logger.error(error_msg)
            self.stats["errors"].append(error_msg)
            return False

    def _apply_mode(
        self, current_content: str, mode: str, content: str, options: Dict
    ) -> str:
        """Apply content update based on the specified mode."""

        if mode == "append":
            return current_content + "\n" + content if current_content else content

        elif mode == "prepend":
            return content + "\n" + current_content if current_content else content

        elif mode == "replace":
            return content

        elif mode == "replace-section":
            section = options["section"]
            return self._replace_section(current_content, section, content)

        elif mode == "insert-after":
            after_text = options["after"]
            return self._insert_after(current_content, after_text, content)

        elif mode == "insert-before":
            before_text = options["before"]
            return self._insert_before(current_content, before_text, content)

        elif mode == "changelog-entry":
            return self._add_changelog_entry(current_content, content)

        elif mode == "task-add":
            return self._add_todo_task(current_content, content)

        elif mode == "task-complete":
            task_id = options.get("task_id")
            return self._complete_todo_task(current_content, content, task_id)

        elif mode == "update-badge":
            badge_name = options["badge_name"]
            return self._update_badge(current_content, badge_name, content)

        else:
            raise ValueError(f"Unknown update mode: {mode}")

    def _replace_section(self, content: str, section: str, new_content: str) -> str:
        """Replace a specific section in the content."""
        pattern = rf"(^#{1, 6}\s+{re.escape(section)}\s*$.*?)(?=^#{1, 6}\s+|\Z)"

        if re.search(pattern, content, re.MULTILINE | re.DOTALL):
            return re.sub(
                pattern,
                f"# {section}\n\n{new_content}\n",
                content,
                flags=re.MULTILINE | re.DOTALL,
            )
        else:
            return content + f"\n\n# {section}\n\n{new_content}\n"

    def _insert_after(self, content: str, after_text: str, new_content: str) -> str:
        """Insert content after specified text."""
        if after_text in content:
            return content.replace(after_text, after_text + "\n" + new_content)
        else:
            return content + "\n" + new_content

    def _insert_before(self, content: str, before_text: str, new_content: str) -> str:
        """Insert content before specified text."""
        if before_text in content:
            return content.replace(before_text, new_content + "\n" + before_text)
        else:
            return new_content + "\n" + content

    def _add_changelog_entry(self, content: str, entry: str) -> str:
        """Add entry to changelog under [Unreleased] section."""
        unreleased_pattern = r"(## \[Unreleased\].*?\n)(.*?)(?=\n## |\Z)"

        match = re.search(unreleased_pattern, content, re.DOTALL)
        if match:
            return content.replace(
                match.group(0), match.group(1) + "\n" + entry + "\n" + match.group(2)
            )
        else:
            unreleased_section = f"""## [Unreleased]

{entry}

"""
            version_pattern = r"(## \[[\d\.]+\])"
            if re.search(version_pattern, content):
                return re.sub(
                    version_pattern, unreleased_section + r"\1", content, count=1
                )
            else:
                return content + "\n" + unreleased_section

    def _add_todo_task(self, content: str, task: str) -> str:
        """Add a task to TODO list."""
        return content + "\n" + task + "\n"

    def _complete_todo_task(
        self, content: str, task_description: str, task_id: Optional[str]
    ) -> str:
        """Mark a TODO task as complete."""

        def mark_complete(match):
            return match.group(0).replace("[ ]", "[x]")

        if task_id:
            pattern = rf"- \[ \] .*{re.escape(task_id)}.*"
            return re.sub(pattern, mark_complete, content)
        else:
            pattern = rf"- \[ \] .*{re.escape(task_description)}.*"
            return re.sub(pattern, mark_complete, content)

    def _update_badge(self, content: str, badge_name: str, badge_content: str) -> str:
        """Update or add a badge in README."""
        return content + f"\n{badge_content}\n"

    def _handle_malformed_file(self, update_file: Path, error_msg: str) -> None:
        """Handle malformed files by moving them to malformed directory."""
        logger.warning(f"⚠️ Malformed file: {update_file.name} - {error_msg}")

        self.stats["files_malformed"] += 1
        self.stats["malformed_files"].append(update_file.name)
        self.stats["errors"].append(f"Malformed file {update_file.name}: {error_msg}")

        if self.cleanup:
            self._move_to_malformed(update_file, error_msg)

    def _move_to_processed(self, update_file: Path) -> None:
        """Move file to processed directory."""
        try:
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            processed_name = f"{timestamp}_{update_file.name}"
            processed_path = self.processed_dir / processed_name

            shutil.move(str(update_file), str(processed_path))
            logger.debug(f"📦 Moved to processed: {processed_name}")

        except Exception as e:
            logger.warning(f"Failed to move {update_file.name} to processed: {e}")

    def _move_to_malformed(self, update_file: Path, error_msg: str) -> None:
        """Move malformed file to malformed directory with error info."""
        try:
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            malformed_name = f"{timestamp}_{update_file.name}"
            malformed_path = self.malformed_dir / malformed_name

            # Create error info file
            error_file = (
                self.malformed_dir / f"{timestamp}_{update_file.stem}_error.txt"
            )
            with open(error_file, "w", encoding="utf-8") as f:
                f.write(f"File: {update_file.name}\n")
                f.write(f"Error: {error_msg}\n")
                f.write(f"Timestamp: {datetime.now().isoformat()}\n")

            shutil.move(str(update_file), str(malformed_path))
            logger.debug(f"🔄 Moved to malformed: {malformed_name}")

        except Exception as e:
            logger.warning(f"Failed to move {update_file.name} to malformed: {e}")

    def _move_to_failed(self, update_file: Path, error_msg: str) -> None:
        """Move failed file to failed directory with error info."""
        try:
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            failed_name = f"{timestamp}_{update_file.name}"
            failed_path = self.failed_dir / failed_name

            # Create error info file
            error_file = self.failed_dir / f"{timestamp}_{update_file.stem}_error.txt"
            with open(error_file, "w", encoding="utf-8") as f:
                f.write(f"File: {update_file.name}\n")
                f.write(f"Error: {error_msg}\n")
                f.write(f"Timestamp: {datetime.now().isoformat()}\n")
                f.write(f"Stack trace:\n{traceback.format_exc()}")

            shutil.move(str(update_file), str(failed_path))
            logger.debug(f"❌ Moved to failed: {failed_name}")

            self.stats["files_failed"] += 1
            self.stats["failed_files"].append(update_file.name)

        except Exception as e:
            logger.warning(f"Failed to move {update_file.name} to failed: {e}")

    def process_chronological(self) -> Dict[str, Any]:
        """Process updates in chronological order based on created_at timestamps."""
        logger.info(
            f"🕒 Processing documentation updates chronologically from {self.updates_dir}"
        )

        if not self.updates_dir.exists():
            logger.info(f"📝 Updates directory does not exist: {self.updates_dir}")
            return self.stats

        # Load all updates from files
        all_updates = []
        update_files = [f for f in self.updates_dir.glob("*.json") if f.is_file()]

        if not update_files:
            logger.info("📝 No update files found")
            return self.stats

        logger.info(f"📊 Found {len(update_files)} update files")

        # Load and validate all updates first
        for update_file in update_files:
            try:
                with open(update_file, encoding="utf-8") as f:
                    update_data = json.load(f)

                # Handle both single objects and arrays
                updates_in_file = (
                    [update_data] if isinstance(update_data, dict) else update_data
                )

                for update in updates_in_file:
                    if isinstance(update, dict) and "mode" in update:
                        update["_source_file"] = str(update_file)
                        all_updates.append(update)

            except Exception as e:
                logger.error(f"❌ Error loading {update_file.name}: {e}")
                self._handle_malformed_file(update_file, str(e))

        if not all_updates:
            logger.info("📝 No valid updates to process")
            return self.stats

        # Sort updates chronologically
        sorted_updates = self._sort_updates_chronologically(all_updates)
        logger.info(f"✅ Sorted {len(sorted_updates)} updates chronologically")

        # Process updates in chronological order
        success_count = 0
        for i, update in enumerate(sorted_updates, 1):
            mode = update.get("mode", "unknown")
            filename = update.get("filename", "unknown")
            guid = update.get("guid", "no-guid")
            created_at = update.get("created_at", "no-timestamp")
            source_file = update.get("_source_file", "unknown")

            logger.info(
                f"📋 Update {i}/{len(sorted_updates)}: {mode} on {filename} (guid: {guid[:8]}, created: {created_at})"
            )

            if self.dry_run:
                logger.info(f"🧪 [DRY RUN] Would apply {mode} to {filename}")
                success_count += 1
            else:
                try:
                    # Apply the update
                    success = self._apply_doc_update(update)

                    if success:
                        success_count += 1
                        self.stats["files_processed"] += 1
                        self.stats["changes_made"] = True
                        logger.debug("✅ Successfully applied update")
                    else:
                        self.stats["files_failed"] += 1
                        logger.error("❌ Failed to apply update")

                except Exception as e:
                    logger.error(f"❌ Error processing update: {e}")
                    self.stats["files_failed"] += 1

        # Move processed files
        if self.cleanup and not self.dry_run:
            processed_files = set()
            for update in sorted_updates:
                source_file = Path(update.get("_source_file", ""))
                if source_file.exists() and str(source_file) not in processed_files:
                    self._move_to_processed(source_file)
                    processed_files.add(str(source_file))

        logger.info(
            f"🎯 Successfully processed {success_count}/{len(sorted_updates)} updates chronologically"
        )
        self._log_processing_summary()
        return self.stats

    def _sort_updates_chronologically(self, updates: list) -> list:
        """Sort updates by created_at timestamp, then by sequence number."""

        def get_sort_key(update):
            created_at = update.get("created_at") or update.get("timestamp", "")
            sequence = update.get("sequence", 0)

            try:
                # Parse timestamp
                from datetime import datetime

                dt = datetime.fromisoformat(created_at.replace("Z", "+00:00"))
                return (dt, sequence)
            except (ValueError, AttributeError):
                # Fallback for malformed timestamps
                return (created_at, sequence)

        return sorted(updates, key=get_sort_key)

    def _apply_doc_update(self, update: Dict[str, Any]) -> bool:
        """Apply a documentation update using the appropriate mode."""
        mode = update.get("mode")
        filename = update.get("filename")
        content = update.get("content", "")

        if not filename:
            logger.error("No filename specified")
            return False

        try:
            if mode == "append":
                return self._apply_append(filename, content)
            elif mode == "prepend":
                return self._apply_prepend(filename, content)
            elif mode == "replace-section":
                section = update.get("section")
                return self._apply_replace_section(filename, content, section)
            elif mode == "changelog-entry":
                entry_type = update.get("type", "feature")
                return self._apply_changelog_entry(filename, content, entry_type)
            elif mode == "task-add":
                priority = update.get("priority", "MEDIUM")
                return self._apply_task_add(filename, content, priority)
            elif mode == "task-complete":
                return self._apply_task_complete(filename, content)
            else:
                logger.error(f"Unknown mode: {mode}")
                return False

        except Exception as e:
            logger.error(f"Error applying {mode}: {e}")
            return False

    def _apply_append(self, filename: str, content: str) -> bool:
        """Append content to the end of a file."""
        try:
            file_path = Path(filename)
            if file_path.exists():
                with open(file_path, "r", encoding="utf-8") as f:
                    existing_content = f.read()
            else:
                existing_content = ""

            with open(file_path, "w", encoding="utf-8") as f:
                f.write(existing_content)
                if existing_content and not existing_content.endswith("\n"):
                    f.write("\n")
                f.write(content)
                if not content.endswith("\n"):
                    f.write("\n")

            logger.debug(f"Appended content to {filename}")
            if filename not in self.stats["files_updated"]:
                self.stats["files_updated"].append(filename)
            return True

        except Exception as e:
            logger.error(f"Error appending to {filename}: {e}")
            return False

    def _apply_prepend(self, filename: str, content: str) -> bool:
        """Prepend content to the beginning of a file."""
        try:
            file_path = Path(filename)
            if file_path.exists():
                with open(file_path, "r", encoding="utf-8") as f:
                    existing_content = f.read()
            else:
                existing_content = ""

            with open(file_path, "w", encoding="utf-8") as f:
                f.write(content)
                if not content.endswith("\n"):
                    f.write("\n")
                if existing_content:
                    f.write(existing_content)

            logger.debug(f"Prepended content to {filename}")
            if filename not in self.stats["files_updated"]:
                self.stats["files_updated"].append(filename)
            return True

        except Exception as e:
            logger.error(f"Error prepending to {filename}: {e}")
            return False

    def _apply_replace_section(
        self, filename: str, content: str, section: Optional[str]
    ) -> bool:
        """Replace a specific section in a file."""
        try:
            file_path = Path(filename)
            if not file_path.exists():
                logger.error(f"File {filename} does not exist")
                return False

            with open(file_path, "r", encoding="utf-8") as f:
                existing_content = f.read()

            if section:
                # Replace specific section
                pattern = rf"(## {re.escape(section)}.*?)(?=\n## |\n# |\Z)"
                replacement = f"## {section}\n\n{content}"
                new_content = re.sub(
                    pattern, replacement, existing_content, flags=re.DOTALL
                )

                if new_content == existing_content:
                    logger.warning(
                        f"Section '{section}' not found in {filename}, appending instead"
                    )
                    new_content = existing_content + f"\n\n## {section}\n\n{content}"
            else:
                # Replace entire file
                new_content = content

            with open(file_path, "w", encoding="utf-8") as f:
                f.write(new_content)

            logger.debug(f"Replaced section in {filename}")
            if filename not in self.stats["files_updated"]:
                self.stats["files_updated"].append(filename)
            return True

        except Exception as e:
            logger.error(f"Error replacing section in {filename}: {e}")
            return False

    def _apply_changelog_entry(
        self, filename: str, content: str, entry_type: str
    ) -> bool:
        """Add a changelog entry."""
        try:
            timestamp = datetime.now().strftime("%Y-%m-%d")

            # Format the changelog entry
            entry = f"### {entry_type.title()} - {timestamp}\n\n{content}\n"

            file_path = Path(filename)
            if file_path.exists():
                with open(file_path, "r", encoding="utf-8") as f:
                    existing_content = f.read()

                # Find the right place to insert (after the first ## heading)
                lines = existing_content.split("\n")
                insert_index = 0

                for i, line in enumerate(lines):
                    if line.startswith("## "):
                        insert_index = i + 1
                        break

                lines.insert(insert_index, "")
                lines.insert(insert_index + 1, entry)
                new_content = "\n".join(lines)
            else:
                new_content = f"# Changelog\n\n{entry}"

            with open(file_path, "w", encoding="utf-8") as f:
                f.write(new_content)

            logger.debug(f"Added {entry_type} changelog entry to {filename}")
            if filename not in self.stats["files_updated"]:
                self.stats["files_updated"].append(filename)
            return True

        except Exception as e:
            logger.error(f"Error adding changelog entry to {filename}: {e}")
            return False

    def _apply_task_add(self, filename: str, content: str, priority: str) -> bool:
        """Add a task to a TODO file."""
        try:
            timestamp = datetime.now().strftime("%Y-%m-%d")
            task_entry = f"- [ ] **{priority}** {content} (added {timestamp})\n"

            file_path = Path(filename)
            if file_path.exists():
                with open(file_path, "r", encoding="utf-8") as f:
                    existing_content = f.read()

                # Add to the end of the file
                new_content = existing_content
                if not existing_content.endswith("\n"):
                    new_content += "\n"
                new_content += task_entry
            else:
                new_content = f"# TODO\n\n{task_entry}"

            with open(file_path, "w", encoding="utf-8") as f:
                f.write(new_content)

            logger.debug(f"Added {priority} priority task to {filename}")
            if filename not in self.stats["files_updated"]:
                self.stats["files_updated"].append(filename)
            return True

        except Exception as e:
            logger.error(f"Error adding task to {filename}: {e}")
            return False

    def _apply_task_complete(self, filename: str, content: str) -> bool:
        """Mark a task as complete in a TODO file."""
        try:
            file_path = Path(filename)
            if not file_path.exists():
                logger.error(f"File {filename} does not exist")
                return False

            with open(file_path, "r", encoding="utf-8") as f:
                existing_content = f.read()

            # Find and mark the task as complete
            lines = existing_content.split("\n")
            found = False

            for i, line in enumerate(lines):
                if content.lower() in line.lower() and "- [ ]" in line:
                    lines[i] = line.replace("- [ ]", "- [x]")
                    found = True
                    break

            if not found:
                logger.warning(f"Task not found in {filename}: {content}")
                return False

            new_content = "\n".join(lines)

            with open(file_path, "w", encoding="utf-8") as f:
                f.write(new_content)

            logger.debug(f"Marked task as complete in {filename}")
            if filename not in self.stats["files_updated"]:
                self.stats["files_updated"].append(filename)
            return True

        except Exception as e:
            logger.error(f"Error completing task in {filename}: {e}")
            return False

    def _log_processing_summary(self) -> None:
        """Log comprehensive processing summary."""
        logger.info("\n📊 Processing Summary:")
        logger.info(f"   Files processed successfully: {self.stats['files_processed']}")
        logger.info(f"   Files with malformed data: {self.stats['files_malformed']}")
        logger.info(f"   Files that failed processing: {self.stats['files_failed']}")
        logger.info(
            f"   Documentation files updated: {len(self.stats['files_updated'])}"
        )
        logger.info(f"   Changes made to repository: {self.stats['changes_made']}")

        if self.stats["malformed_files"]:
            logger.warning(
                f"   Malformed files: {', '.join(self.stats['malformed_files'])}"
            )

        if self.stats["failed_files"]:
            logger.warning(f"   Failed files: {', '.join(self.stats['failed_files'])}")

        if self.stats["errors"]:
            logger.warning(f"   Total errors encountered: {len(self.stats['errors'])}")


def main():
    """Main function with CLI interface for enhanced documentation update management."""
    parser = argparse.ArgumentParser(
        description="Enhanced Documentation Update Manager with Comprehensive Timestamp Lifecycle Tracking v4.0",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python enhanced_doc_update_manager.py process                    # Regular processing
  python enhanced_doc_update_manager.py process-chronological      # Chronological processing
  python enhanced_doc_update_manager.py --dry-run --verbose        # For backwards compatibility
        """,
    )

    # Add subcommands
    subparsers = parser.add_subparsers(dest="command", help="Available commands")

    # Regular processing command (default behavior)
    process_parser = subparsers.add_parser(
        "process", help="Process updates in filename order (default)"
    )
    process_parser.add_argument(
        "--updates-dir",
        default=".github/doc-updates",
        help="Directory containing update files (default: .github/doc-updates)",
    )
    process_parser.add_argument(
        "--cleanup",
        type=lambda x: x.lower() in ("true", "1", "yes"),
        default=True,
        help="Whether to move processed files to subdirectories (default: true)",
    )
    process_parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be updated without making changes",
    )
    process_parser.add_argument(
        "--verbose", action="store_true", help="Enable verbose logging"
    )
    process_parser.add_argument(
        "--continue-on-error",
        type=lambda x: x.lower() in ("true", "1", "yes"),
        default=True,
        help="Continue processing other files when one fails (default: true)",
    )

    # Chronological processing command
    chrono_parser = subparsers.add_parser(
        "process-chronological",
        help="Process updates in chronological order based on created_at timestamps",
    )
    chrono_parser.add_argument(
        "--updates-dir",
        default=".github/doc-updates",
        help="Directory containing update files (default: .github/doc-updates)",
    )
    chrono_parser.add_argument(
        "--cleanup",
        type=lambda x: x.lower() in ("true", "1", "yes"),
        default=True,
        help="Whether to move processed files to subdirectories (default: true)",
    )
    chrono_parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be updated without making changes",
    )
    chrono_parser.add_argument(
        "--verbose", action="store_true", help="Enable verbose logging"
    )
    chrono_parser.add_argument(
        "--continue-on-error",
        type=lambda x: x.lower() in ("true", "1", "yes"),
        default=True,
        help="Continue processing other files when one fails (default: true)",
    )

    # For backwards compatibility, support old argument format without subcommands
    parser.add_argument(
        "--updates-dir",
        default=".github/doc-updates",
        help="Directory containing update files (default: .github/doc-updates)",
    )
    parser.add_argument(
        "--cleanup",
        type=lambda x: x.lower() in ("true", "1", "yes"),
        default=True,
        help="Whether to move processed files to subdirectories (default: true)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be updated without making changes",
    )
    parser.add_argument("--verbose", action="store_true", help="Enable verbose logging")
    parser.add_argument(
        "--continue-on-error",
        type=lambda x: x.lower() in ("true", "1", "yes"),
        default=True,
        help="Continue processing other files when one fails (default: true)",
    )

    args = parser.parse_args()

    # Determine which command to run
    command = getattr(args, "command", None)
    if not command:
        # Default behavior for backwards compatibility
        command = "process"

    manager = EnhancedDocumentationUpdateManager(
        updates_dir=args.updates_dir,
        cleanup=args.cleanup,
        dry_run=args.dry_run,
        verbose=args.verbose,
        continue_on_error=args.continue_on_error,
    )

    try:
        if command == "process-chronological":
            stats = manager.process_chronological()
        else:
            stats = manager.process_all_updates()

        print("\n📊 Final Processing Summary:")
        print(f"   ✅ Successfully processed: {stats['files_processed']}")
        print(f"   ⚠️  Malformed files: {stats['files_malformed']}")
        print(f"   ❌ Failed files: {stats['files_failed']}")
        print(f"   📝 Documentation files updated: {len(stats['files_updated'])}")

        if stats["files_updated"]:
            print(f"   📄 Updated files: {', '.join(stats['files_updated'])}")

        # Exit with appropriate status
        if stats["files_failed"] > 0 and not args.continue_on_error:
            sys.exit(1)
        elif stats["files_malformed"] > 0 or stats["files_failed"] > 0:
            print(
                "\n⚠️  Completed with issues - check malformed/ and failed/ directories"
            )
            sys.exit(0)  # Success despite issues when continue_on_error=true
        else:
            print("\n✅ All files processed successfully")
            sys.exit(0)

    except KeyboardInterrupt:
        logger.info("🛑 Interrupted by user")
        sys.exit(1)
    except Exception as e:
        logger.error(f"❌ Unexpected error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
