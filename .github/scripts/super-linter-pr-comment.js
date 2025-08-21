// file: .github/scripts/super-linter-pr-comment.js
// version: 1.0.0
// guid: 7e2a1b4c-8d3f-4a6b-9c2e-1f2b3c4d5e6f

/**
 * Script to post Super Linter results as a PR comment.
 * Used by actions/github-script in CI workflows.
 *
 * Usage: Called by GitHub Actions with context, github, core.
 */

module.exports = async function ({ github, context, core }) {
  // Example: Post a comment with linter results
  // You may want to customize this logic for your workflow
  const prNumber = context.payload.pull_request
    ? context.payload.pull_request.number
    : null;
  if (!prNumber) {
    core.info('No pull request found in context. Skipping comment.');
    return;
  }

  // Example linter result (replace with actual linter output)
  const linterOutput =
    process.env.LINTER_RESULTS || 'Super Linter completed. No results found.';

  await github.rest.issues.createComment({
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: prNumber,
    body: `### Super Linter Results\n\n${linterOutput}`,
  });

  core.info('Super Linter PR comment posted.');
};
