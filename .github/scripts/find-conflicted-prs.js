#!/usr/bin/env node
// file: .github/scripts/find-conflicted-prs.js
// version: 1.0.0
// guid: 12345678-abcd-efgh-ijkl-123456789abc

/**
 * Find PRs with merge conflicts and output as JSON
 */

async function findConflictedPRs(github, context, core) {
  const prs = await github.paginate(github.rest.pulls.list, {
    owner: context.repo.owner,
    repo: context.repo.repo,
    state: 'open',
  });

  const conflicted = [];

  for (const pr of prs) {
    let full = await github.rest.pulls.get({
      owner: context.repo.owner,
      repo: context.repo.repo,
      pull_number: pr.number,
    });

    // GitHub may return 'unknown' mergeable_state on first request
    if (full.data.mergeable_state === 'unknown') {
      await new Promise(r => setTimeout(r, 2000));
      full = await github.rest.pulls.get({
        owner: context.repo.owner,
        repo: context.repo.repo,
        pull_number: pr.number,
      });
    }

    const state = full.data.mergeable_state;
    core.info(`PR #${pr.number} state: ${state}`);

    if (state === 'dirty' || state === 'behind') {
      conflicted.push({
        number: pr.number,
        branch: pr.head.ref,
        state: state,
      });
    }
  }

  core.setOutput('conflicted_prs', JSON.stringify(conflicted));
  core.info(`Found ${conflicted.length} conflicted PR(s)`);

  return conflicted;
}

module.exports = { findConflictedPRs };

// For GitHub Actions script execution
if (require.main === module) {
  // This would be called from GitHub Actions
  console.log(
    'This script should be called from GitHub Actions with proper context'
  );
}
