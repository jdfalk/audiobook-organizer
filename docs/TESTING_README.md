<!-- file: docs/TESTING_README.md -->
<!-- version: 1.0.0 -->
<!-- guid: f6a7b8c9-d0e1-2345-f123-456789012345 -->

# Testing Documentation Index

## Overview

This directory contains comprehensive manual testing documentation for the
audiobook-organizer project. The documentation was created to support PR #79
(Metadata Provenance feature) merge validation and establish ongoing QA
processes.

**Created**: December 28, 2025 **Status**: Ready for Use **Target**: PR #79
merge validation

---

## Quick Start

### For QA Engineers

**Start Here**: [P0 Test Checklist](./MANUAL_TEST_CHECKLIST_P0.md)

1. Read [Executive Summary](./MANUAL_TESTING_EXECUTIVE_SUMMARY.md) (10 minutes)
2. Set up test environment using
   [Test Data Setup Guide](./TEST_DATA_SETUP_GUIDE.md) (45 minutes)
3. Execute [P0 Test Checklist](./MANUAL_TEST_CHECKLIST_P0.md) (2-3 hours)
4. Report issues using templates in
   [Validation Criteria](./TEST_VALIDATION_CRITERIA.md)
5. Complete test sign-off

**Total Time**: ~4 hours

---

### For Project Managers

**Start Here**: [Executive Summary](./MANUAL_TESTING_EXECUTIVE_SUMMARY.md)

Read these sections:

- Executive Summary (5 minutes)
- Testing Scope (5 minutes)
- Risk Assessment (5 minutes)
- Recommendations (5 minutes)

**Total Time**: ~20 minutes

---

### For Developers

**Start Here**: [Manual Test Plan](./MANUAL_TEST_PLAN.md)

Use cases:

- Reference for expected behavior during development
- Validation after fixing bugs
- Understanding test scenarios for new features

**Key Sections**:

- Test scenarios for your feature area
- API validation commands
- Browser console debugging commands

---

## Document Structure

### 📋 [Manual Test Plan](./MANUAL_TEST_PLAN.md)

**Purpose**: Comprehensive test scenario repository

**Content**:

- 30+ detailed test scenarios across 6 feature areas
- Step-by-step instructions with screenshots guidance
- Expected results and validation checkpoints
- Accessibility and performance testing
- Error handling scenarios
- Test data cleanup procedures

**When to Use**:

- Comprehensive testing campaigns
- Feature development reference
- Regression testing
- Training new QA team members

**Size**: ~28,000 words, 94 pages equivalent

---

### ✅ [P0 Test Checklist](./MANUAL_TEST_CHECKLIST_P0.md)

**Purpose**: Critical tests required before PR #79 merge

**Content**:

- 10 must-pass test scenarios
- Checkbox format for easy tracking
- Quick reference API commands
- Test summary and sign-off section
- Pass/fail tracking table

**When to Use**:

- PR merge gate validation
- Quick smoke testing after fixes
- Release readiness verification

**Size**: ~5,500 words, 18 pages equivalent

**Estimated Execution Time**: 2-3 hours

---

### 🎯 [Test Data Setup Guide](./TEST_DATA_SETUP_GUIDE.md)

**Purpose**: Instructions for creating test audiobook files

**Content**:

- FFmpeg-based test file generation
- Multiple approaches (real files, synthetic, public domain)
- Metadata tagging examples
- Hash computation and management
- Test file verification scripts
- CI/CD integration examples

**When to Use**:

- Setting up new test environment
- Preparing for manual testing session
- Automating test data creation
- CI/CD pipeline configuration

**Size**: ~8,500 words, 28 pages equivalent

---

### 📊 [Test Validation Criteria](./TEST_VALIDATION_CRITERIA.md)

**Purpose**: Objective pass/fail criteria and issue reporting

**Content**:

- Detailed pass criteria for all test scenarios
- Severity thresholds (Critical/High/Medium/Low)
- Issue reporting templates (Bug, Test Failure, Enhancement)
- Accessibility validation criteria
- Performance benchmarks
- Test sign-off checklist

**When to Use**:

- Evaluating test results consistently
- Reporting issues with proper context
- Triaging bugs by severity
- Making merge/release decisions

**Size**: ~9,500 words, 32 pages equivalent

---

### 📈 [Executive Summary](./MANUAL_TESTING_EXECUTIVE_SUMMARY.md)

**Purpose**: High-level overview for decision-makers

**Content**:

- Project status and PR #79 overview
- Risk assessment matrix
- Testing approach recommendations
- Decision matrix for merge strategies
- Timeline estimates
- Resource requirements
- FAQ

**When to Use**:

- Presenting testing status to stakeholders
- Making merge/release decisions
- Resource allocation planning
- Understanding testing ROI

**Size**: ~8,000 words, 26 pages equivalent

---

## Total Documentation

**Word Count**: ~51,500 words **Page Equivalent**: ~172 pages **Creation Time**:
~8 hours **Maintenance**: Update as features evolve

---

## Testing Workflow

### Phase 1: Preparation (45 minutes)

1. **Environment Setup** (15 minutes)
   - Start application locally
   - Verify database clean state
   - Check browser developer tools ready

2. **Test Data Preparation** (30 minutes)
   - Follow [Test Data Setup Guide](./TEST_DATA_SETUP_GUIDE.md)
   - Generate or copy test audiobook files
   - Compute and document file hashes
   - Verify files with validation script

---

### Phase 2: Critical Testing (2-3 hours)

1. **Execute P0 Checklist**
   - Follow [P0 Test Checklist](./MANUAL_TEST_CHECKLIST_P0.md)
   - Check off each scenario as completed
   - Document any issues found
   - Collect screenshots/logs as evidence

2. **Issue Reporting**
   - Use templates from [Validation Criteria](./TEST_VALIDATION_CRITERIA.md)
   - Assign severity to each issue
   - Create GitHub issues for tracking

---

### Phase 3: Review and Sign-Off (30 minutes)

1. **Results Analysis**
   - Calculate pass rate (X/10 tests)
   - Review critical/high severity issues
   - Determine merge blocker status

2. **Documentation**
   - Complete test summary section in checklist
   - Provide merge recommendation
   - Obtain tech lead sign-off

---

## Test Coverage Map

### PR #79: Metadata Provenance

**Feature**: Per-field metadata source tracking and manual overrides

**P0 Tests**:

- ✅ Test 1: Provenance data display
- ✅ Test 2: Apply override from file value
- ✅ Test 3: Clear override and fallback
- ✅ Test 4: Lock toggle persistence

**E2E Tests**: 13 automated scenarios in
[metadata-provenance.spec.ts](../web/tests/e2e/metadata-provenance.spec.ts)

**Documentation**: See
[Metadata Provenance Tests](../web/tests/e2e/METADATA_PROVENANCE_TESTS.md)

---

### PR #69: Blocked Hashes Management

**Feature**: Prevent reimporting deleted audiobooks by hash

**P0 Tests**:

- ✅ Test 5: Add blocked hash with validation
- ✅ Test 6: Delete blocked hash with confirmation

**UI Component**: `web/src/components/settings/BlockedHashesTab.tsx`

**Documentation**: See [CHANGELOG.md](../CHANGELOG.md) - December 22, 2025 entry

---

### PR #70: State Transitions & Delete Flows

**Feature**: Book lifecycle tracking and soft delete with purge

**P0 Tests**:

- ✅ Test 7: Soft delete without hash blocking
- ✅ Test 8: Soft delete with hash blocking
- ✅ Test 9: Restore soft-deleted book
- ✅ Test 10: State transition (import → organized)

**Database**: Migration 9 - State machine fields

**Documentation**: See [CHANGELOG.md](../CHANGELOG.md) - December 22-23, 2025
entries

---

## Best Practices

### Before Testing

- [ ] Read relevant test documentation fully
- [ ] Understand pass/fail criteria
- [ ] Prepare test environment and data
- [ ] Have issue templates ready
- [ ] Clear browser cache

### During Testing

- [ ] Follow test steps exactly as written
- [ ] Take screenshots of issues
- [ ] Copy error messages and logs
- [ ] Test both happy path and edge cases
- [ ] Note any usability concerns

### After Testing

- [ ] Document all issues found (even minor)
- [ ] Complete test summary section
- [ ] Provide clear merge recommendation
- [ ] Archive test evidence
- [ ] Update documentation if needed

---

## Common Pitfalls

### ❌ Don't

- Skip test data setup (leads to invalid test results)
- Test in production environment
- Rush through test scenarios
- Ignore minor issues (may indicate larger problems)
- Test without clear pass/fail criteria

### ✅ Do

- Follow test steps exactly
- Document everything (issues, observations, timings)
- Use provided templates for consistency
- Communicate blockers immediately
- Suggest improvements based on testing experience

---

## Issue Severity Quick Reference

| Severity | Definition                      | Example                          | Action             |
| -------- | ------------------------------- | -------------------------------- | ------------------ |
| Critical | Blocks release, data loss       | Can't save override, app crashes | Fix immediately    |
| High     | Major feature broken            | UI doesn't update, wrong values  | Fix before release |
| Medium   | Minor feature issue, workaround | Slow loading, cosmetic bugs      | Fix next sprint    |
| Low      | Polish, rare edge case          | Tooltip typo, minor spacing      | Tech debt          |

---

## Frequently Asked Questions

### Q: Which document should I read first?

**A**: Depends on your role:

- **QA Engineer**: P0 Test Checklist
- **Project Manager**: Executive Summary
- **Developer**: Manual Test Plan (your feature area)
- **New Team Member**: Executive Summary → Manual Test Plan

---

### Q: How long does manual testing take?

**A**:

- **P0 Critical Tests**: 2-3 hours execution + 1 hour setup/reporting = ~4 hours
  total
- **Full Manual Suite**: 6-8 hours execution + 1.5 hours setup/reporting = ~10
  hours total

---

### Q: Can automated tests replace manual testing?

**A**: No. Automated tests validate logic and basic interactions. Manual testing
validates:

- Visual design and layout
- User experience flow
- Error messages and feedback
- Accessibility (keyboard, screen reader)
- Cross-browser compatibility
- Real-world usability

---

### Q: What if I find a critical bug?

**A**:

1. Stop testing that feature area
2. Document the issue using Bug Report template
3. Notify tech lead immediately
4. Mark as merge blocker if appropriate
5. Wait for fix before continuing related tests

---

### Q: How often should we run manual tests?

**A**:

- **P0 Tests**: Before each major PR merge
- **Full Manual Suite**: Before each release
- **Regression Tests**: After bug fixes
- **Exploratory Testing**: Ongoing, especially for new features

---

## Test Metrics

Track these metrics to improve testing process:

| Metric              | Target       | How to Measure                    |
| ------------------- | ------------ | --------------------------------- |
| Test execution time | <4 hours     | Track start/end time              |
| Pass rate           | >90%         | Passed tests / Total tests        |
| Issues found        | Document all | Count by severity                 |
| Fix time            | <1 day       | Time from report to fix           |
| Regression rate     | <5%          | Previously passing tests now fail |

---

## Version Control

All testing documentation is version-controlled in git. When updating:

1. Increment version number in file header
2. Update `Version History` section
3. Document what changed in commit message
4. Update this README if structure changes

---

## Contributing

To improve testing documentation:

1. Execute tests and note pain points
2. Suggest clarifications or additions
3. Submit PR with documentation updates
4. Include rationale for changes

**Maintainer**: Documentation Curator Agent

---

## Related Resources

### Project Documentation

- [CHANGELOG.md](../CHANGELOG.md) - Version history
- [TODO.md](../TODO.md) - Outstanding tasks
- [NEXT_STEPS.md](../NEXT_STEPS.md) - Upcoming priorities
- [SESSION_SUMMARY.md](../SESSION_SUMMARY.md) - Recent work

### E2E Test Documentation

- [Test Coverage Summary](../web/tests/e2e/TEST_COVERAGE_SUMMARY.md)
- [Metadata Provenance Tests](../web/tests/e2e/METADATA_PROVENANCE_TESTS.md)
- [E2E Next Steps](../web/tests/e2e/NEXT_STEPS.md)

### Application Documentation

- [QUICKSTART.md](../QUICKSTART.md) - Quick start guide
- [README.md](../README.md) - Project overview

---

## Support

**Questions about testing?**

- Check FAQ section above
- Review relevant test documentation
- Consult with QA lead or tech lead

**Found documentation issues?**

- Create GitHub issue with label `documentation`
- Suggest improvements in PR review
- Update directly if you have write access

---

## Version History

- **1.0.0** (2025-12-28): Initial testing documentation index created
  - Created comprehensive testing documentation package
  - Established testing workflow and best practices
  - Documented all test coverage areas

---

**Status**: ✅ Documentation Complete

**Next Action**: Execute [P0 Test Checklist](./MANUAL_TEST_CHECKLIST_P0.md) for
PR #79 validation

---

## Document Map

```
docs/
├── TESTING_README.md (this file)           - Documentation index
├── MANUAL_TESTING_EXECUTIVE_SUMMARY.md     - Decision-maker overview
├── MANUAL_TEST_PLAN.md                     - Comprehensive scenarios
├── MANUAL_TEST_CHECKLIST_P0.md             - Critical merge gate tests
├── TEST_DATA_SETUP_GUIDE.md                - Environment preparation
└── TEST_VALIDATION_CRITERIA.md             - Pass/fail standards
```

**Total**: 6 documents, ~60,000 words, ~200 pages
