#!/usr/bin/env node
// file: .github/scripts/super-linter-pr-comment.js
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-123456789012

/**
 * Super Linter PR Comment Script
 * Creates or updates PR comments with linting results and auto-fix information
 */

const fs = require('fs');

module.exports = async ({ github, context, core }) => {
  // Check if there are any linting errors
  let hasErrors = false;
  let summary = '## 🔍 Super Linter Results\n\n';

  // Get input parameters from environment
  const hasAutoFixes = process.env.HAS_AUTO_FIXES === 'true';
  const autoFixEnabled = process.env.AUTO_FIX_ENABLED === 'true';
  const autoCommitEnabled = process.env.AUTO_COMMIT_ENABLED === 'true';

  if (autoFixEnabled) {
    if (hasAutoFixes && autoCommitEnabled) {
      summary += '🔧 **Auto-fixes applied and committed!**\n\n';
      summary += 'The following issues were automatically fixed:\n';
      summary += '- Code formatting (Black, Prettier, etc.)\n';
      summary += '- Import sorting and organization\n';
      summary += '- Basic syntax and style issues\n\n';
    } else if (hasAutoFixes && !autoCommitEnabled) {
      summary += '🔧 **Auto-fixes available but not committed**\n\n';
      summary +=
        'Auto-fixes were applied but not committed due to configuration.\n\n';
    } else if (!hasAutoFixes) {
      summary += '✨ **No auto-fixes needed**\n\n';
    }
  }

  try {
    // Try different locations for the report file
    let reportContent = '';
    let reportFound = false;

    const reportPaths = [
      'super-linter-reports/super-linter.report',
      'super-linter.report',
      'super-linter-reports/super-linter.log',
    ];

    for (const reportPath of reportPaths) {
      if (fs.existsSync(reportPath)) {
        reportContent = fs.readFileSync(reportPath, 'utf8');
        reportFound = true;
        core.info(`Found report at: ${reportPath}`);
        break;
      }
    }

    if (reportFound) {
      if (reportContent.includes('ERROR') || reportContent.includes('FATAL')) {
        hasErrors = true;
        summary += '❌ **Linting failed** - Please fix the issues below:\n\n';
        // Truncate very long reports
        if (reportContent.length > 4000) {
          reportContent =
            reportContent.substring(0, 4000) + '\n... (truncated)';
        }
        summary += '```\n' + reportContent + '\n```\n\n';
      } else {
        summary += '✅ **All linting checks passed!**\n\n';
      }
    } else {
      summary += '⚠️ **No linting report found**\n\n';
      core.warning('No linting report found in any expected location');
    }
  } catch (error) {
    summary +=
      '⚠️ **Error reading linting results**: ' + error.message + '\n\n';
    core.error(`Error reading linting results: ${error.message}`);
  }

  if (autoFixEnabled) {
    summary += '### Auto-fix Configuration\n';
    summary += `- **Auto-fix enabled**: ${autoFixEnabled ? '✅' : '❌'}\n`;
    summary += `- **Auto-commit enabled**: ${autoCommitEnabled ? '✅' : '❌'}\n`;
    summary +=
      '- **Supported formatters**: Black (Python), Prettier (JS/TS), stylelint (CSS), markdownlint, yamllint, gofmt\n\n';
  }

  summary +=
    'View the [workflow run](' +
    context.payload.repository.html_url +
    '/actions/runs/' +
    context.runId +
    ') for detailed results.';

  try {
    // Find existing comment
    const comments = await github.rest.issues.listComments({
      owner: context.repo.owner,
      repo: context.repo.repo,
      issue_number: context.issue.number,
    });

    const existingComment = comments.data.find(comment =>
      comment.body.includes('🔍 Super Linter Results')
    );

    if (existingComment) {
      // Update existing comment
      await github.rest.issues.updateComment({
        owner: context.repo.owner,
        repo: context.repo.repo,
        comment_id: existingComment.id,
        body: summary,
      });
      core.info('Updated existing PR comment');
    } else {
      // Create new comment
      await github.rest.issues.createComment({
        owner: context.repo.owner,
        repo: context.repo.repo,
        issue_number: context.issue.number,
        body: summary,
      });
      core.info('Created new PR comment');
    }
  } catch (error) {
    core.error(`Failed to create/update PR comment: ${error.message}`);
    throw error;
  }
};
