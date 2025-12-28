#!/usr/bin/env node
// file: .github/scripts/process-conflicted-prs.js
// version: 1.0.0
// guid: 87654321-dcba-hgfe-lkji-987654321abc

/**
 * Process conflicted PRs and add helpful comments
 */

async function processConflictedPRs(
  github,
  context,
  conflictedPRs,
  defaultBranch
) {
  for (const pr of conflictedPRs) {
    console.log(
      `Processing conflicted PR #${pr.number} on branch ${pr.branch}`
    );

    // Create a comment about the conflict
    await github.rest.issues.createComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      issue_number: pr.number,
      body: [
        '🤖 **AI Rebase Assistant**',
        '',
        `This PR has merge conflicts (state: ${pr.state}) that need to be resolved. Please:`,
        '',
        `1. Sync your branch with the latest changes from \`${defaultBranch}\``,
        '2. Resolve any conflicts manually',
        '3. Push the updated branch',
        '',
        'For help with resolving conflicts, see: [Resolving Merge Conflicts](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/addressing-merge-conflicts/resolving-a-merge-conflict-using-the-command-line)',
        '',
        '_This comment was generated automatically by the AI Rebase Assistant._',
      ].join('\n'),
    });
  }
}

module.exports = { processConflictedPRs };
