#!/usr/bin/env node
// file: .github/scripts/pr-linter-comment.js
// version: 1.0.0
// guid: abcd1234-efgh-5678-ijkl-abcd12345678

/**
 * Create or update PR comments with linting results
 */

async function updatePRComment(github, context, linterOutcome, hasAutoFixes) {
  // Skip if not a pull request
  if (!context.payload.pull_request) {
    console.log('Not a pull request, skipping summary comment');
    return;
  }

  let summary = '## 🔍 Super Linter Results\n\n';

  // Add concise status
  if (linterOutcome === 'success') {
    summary += '✅ **All code quality checks passed!**\n\n';
  } else if (linterOutcome === 'failure') {
    summary += '❌ **Code quality issues found**\n\n';
  } else {
    summary += '⚠️ **Linter status unknown**\n\n';
  }

  // Show auto-fix information if relevant
  if (hasAutoFixes) {
    summary += '🔧 **Auto-fixes applied and committed**\n\n';
  }

  // For failures, show general guidance
  if (linterOutcome === 'failure') {
    summary += '### Next Steps\n';
    summary += '1. Check the workflow logs for detailed error information\n';
    summary += '2. Fix the issues listed above\n';
    summary += '3. Push changes to update this PR\n\n';
  }

  summary += '*View detailed logs in the workflow run artifacts*';

  // Find and update existing comment or create new one
  const { data: comments } = await github.rest.issues.listComments({
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: context.issue.number,
  });

  const existingComment = comments.find(c =>
    c.user.type === 'Bot' && c.body.includes('Super Linter Results')
  );

  if (existingComment) {
    await github.rest.issues.updateComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      comment_id: existingComment.id,
      body: summary
    });
  } else {
    await github.rest.issues.createComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      issue_number: context.issue.number,
      body: summary
    });
  }
}

module.exports = { updatePRComment };
