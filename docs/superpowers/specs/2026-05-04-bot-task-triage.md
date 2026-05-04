<!-- file: docs/superpowers/specs/2026-05-04-bot-task-triage.md -->
<!-- version: 1.0.0 -->
<!-- guid: c8f3d92a-4e75-4b8c-9d1e-2f3a4b5c6d7e -->
<!-- last-edited: 2026-05-04 -->

# Bot-task triage — 2026-05-04

Triage sweep of `docs/superpowers/bot-tasks/` (141 specs) to separate
already-completed work from open work that's ready to fan out to
sub-agents. Done by the Explore agent on 2026-05-04, evidence-based
(file presence + `git log` matches against the spec's branch / task ID).

Specs marked DONE were moved to `docs/superpowers/bot-tasks/archive/`
in the same change that landed this report. NOT_DONE specs stay in
place and are the queue for `/parallel-sweep`-driven execution.

## DONE — moved to archive

- `2026-04-27-activity-batcher-flush-test.md` — `internal/activity/batcher_test.go` exists, commit 0bdaa053 'feat(activity): add LogBatch/FlushOperation'
- `2026-04-27-bleve-task7-cleanup.md` — commit 6eae3b1f 'fix(series): add ctx cancellation', Bleve cleanup merged
- `2026-04-27-fetch-path-4-17b-metadata-handlers.md` — commit 6ecc6a78 'refactor(server): migrate metadata handlers to httputil'
- `2026-04-27-fetch-path-4-17c-dedup-handlers.md` — commit 6ecc6a78 covers dedup handlers
- `2026-04-27-fetch-path-4-17d-audiobook-handlers.md` — commit 668ac568 'refactor(server): migrate audiobook handlers to httputil'
- `2026-04-27-itunes-tests-4-13a-status.md` — itunes status testing merged
- `2026-04-28-async-core-1-interface.md` — async maintenance interface implemented
- `2026-04-28-async-core-3-discovery.md` — async API client discovery merged
- `2026-04-28-async-w1-2-cleanup-series.md` — commit 733b26f7 'fix(activity): wire LogBatch into maintenance ops' includes series cleanup
- `2026-04-28-async-w2-1-backfill-book-files.md` — async backfill merged
- `2026-04-28-async-w3-1-enrich-book-files.md` — enrich book files ops landed
- `2026-04-28-async-w3-2-dedup-books.md` — dedup books operation implemented
- `2026-04-29-activity-act1-emit-info.md` — commit 733b26f7
- `2026-04-29-activity-act3-isbn-batch-noun.md` — commit 41602586 'chore: act-3 batch noun for isbn-enrich'
- `2026-04-29-audible-category-ladders.md` — Audible category parsing implemented
- `2026-04-29-deluge-1-db-migration.md` — deluge database columns added
- `2026-04-29-deluge-2-protected-path-cache.md` — protected path cache merged
- `2026-04-29-deluge-3-import-to-library.md` — deluge import to library implemented
- `2026-04-29-duration-mismatch-chip.md` — commit 80573385 'feat(maintenance): duration-mismatch detection'
- `2026-04-30-ctx-1-audiobook-update.md` — audiobook update service ctx-refactored
- `2026-04-30-ctx-2-openlibrary.md` — OpenLibrary ctx handler implemented
- `2026-04-30-ctx-3-filesystem-handlers.md` — filesystem ctx handlers merged
- `2026-04-30-db-1-file-hash-index.md` — file hash index added
- `2026-04-30-db-2-begin-tx-sqlite.md` — SQLite tx handling improved
- `2026-04-30-db-3-begin-tx-activity.md` — activity tx semantics fixed
- `2026-04-30-db-4-pipeline-errors.md` — commit 9ee812bf 'fix(database): propagate errors from pipeline save'
- `2026-04-30-fe-3-batch-toolbar.md` — batch toolbar refactored
- `2026-04-30-fe-4-settings-general.md` — settings general tab refactored
- `2026-04-30-fe-5-settings-paths.md` — settings paths tab refactored
- `2026-04-30-fe-6-settings-metadata.md` — settings metadata tab refactored
- `2026-04-30-log-1-tagger.md` — commit 39fad3d7 'fix(scanner): replace fmt.Printf with structured logging in tagger'
- `2026-04-30-log-2-fileops.md` — commit 2c6bd178 'fix(fileops): replace fmt.Printf with structured logging'
- `2026-04-30-log-3-backup.md` — commit b45e6040 'fix(server): replace fmt.Printf in backup'
- `2026-04-30-log-4-progressbar.md` — scanner progress bar removed, structured logging added
- `2026-04-30-n1-1-batch-fetch-interface.md` — batch fetch interface implemented
- `2026-04-30-n1-2-sqlite-impl.md` — SQLite batch fetch implementation added
- `2026-04-30-n1-3-pebble-impl.md` — Pebble batch fetch implementation added
- `2026-04-30-n1-4-enrich-response.md` — batch fetch response enrichment added
- `2026-04-30-sec-2-auth-default.md` — commit 4775bf3c 'fix(security): warn at startup when authentication is disabled'
- `2026-04-30-sec-3-rate-limit-default.md` — commit 9a6612cf 'fix(security): remove duplicate rate-limit middleware'
- `2026-04-30-sec-4-ratelimit-cleanup.md` — rate limit cleanup merged
- `2026-04-30-srv-1-gzip.md` — commit 249ecfc0 'perf(server): add gzip compression middleware'
- `2026-04-30-srv-2-sse-heartbeat.md` — SSE heartbeat feature implemented
- `2026-05-01-ctx-4-activity-store.md` — activity store ctx handler merged
- `2026-05-01-dead-1-remove-unused-code.md` — dead code cleanup completed
- `2026-05-01-dep-1-migrate-itunes-path-field.md` — meta-task; all sub-tasks (a–d) completed
- `2026-05-01-dep-1a-metafetch-itunes-path.md` — metafetch ITunesPath migration done
- `2026-05-01-dep-1b-organizer-itunes-path.md` — organizer ITunesPath migration done
- `2026-05-01-dep-1c-server-itunes-path.md` — server ITunesPath migration done
- `2026-05-01-dep-1d-itunes-service-path.md` — itunes service ITunesPath migration done
- `2026-05-01-perf-1-paginate-getallbooks.md` — GetAllBooks pagination added
- `2026-05-01-pkg-1-audiobooks-service.md` — audiobooks service package extracted
- `2026-05-01-pkg-2-aiscan-pipeline.md` — aiscan pipeline package refactored
- `2026-05-01-pkg-3-reconcile-split.md` — reconcile package split completed
- `2026-05-01-pkg-4-service-packages.md` — service packages refactored
- `2026-05-01-struct-1-server-response-helpers.md` — server response helpers extracted
- `2026-05-01-struct-10-narrow-server-interfaces.md` — server interfaces narrowed
- `2026-05-01-struct-2-pagination-helper.md` — pagination helper extracted
- `2026-05-01-struct-3-maintenance-fixups-split.md` — maintenance fixups split
- `2026-05-01-struct-4-metafetch-service-split.md` — metafetch service split
- `2026-05-01-struct-5-ai-retry-helper.md` — AI retry helper extracted
- `2026-05-01-struct-6-sqlite-store-split.md` — SQLite store split
- `2026-05-01-struct-7-server-go-split.md` — server.go split completed
- `2026-05-01-struct-8-use-async-action-hook.md` — async action hook refactored
- `2026-05-01-test-1-fix-audiobook-service-tests.md` — audiobook service tests fixed
- `2026-05-02-dedup-pipeline-p0-03-fingerprint-store-sqlite-impl.md` — fingerprint SQLite impl merged
- `2026-05-02-dedup-pipeline-p0-04-signal-store-schema-and-impl.md` — signal store schema added
- `2026-05-02-dedup-pipeline-p1-01-stage-sha256-full.md` — SHA256 dedup stage implemented
- `2026-05-02-dedup-pipeline-p1-05-stage-tag-match.md` — tag match dedup stage merged
- `2026-05-02-dedup-pipeline-p1-06-stage-filename-match.md` — filename match stage merged
- `2026-05-02-dedup-pipeline-p3-01-pipeline-coordinator-job.md` — pipeline coordinator job merged
- `2026-05-02-dedup-pipeline-p6-01-backfill-existing-library.md` — dedup backfill for existing library merged

## NOT_DONE — ready to dispatch

- `2026-04-27-activity-batcher-scanner-convert.md` — no scanner-use-logbatch commits
- `2026-04-27-deluge-centralization.md` — no feat/3-1 commits
- `2026-04-27-deluge-undo.md` — no feat/3-2 commits
- `2026-04-27-fetch-path-4-17a-bulk-fetch.md` — no bulk-fetch-delegate commits
- `2026-04-27-itunes-tests-4-13b-track-provisioner.md` — track_provisioner_test.go missing
- `2026-04-27-itunes-tests-4-13c-validate.md` — validate_test.go missing
- `2026-04-27-itunes-tests-4-13d-importer.md` — itunes importer error tests not found
- `2026-04-27-itunes-tests-4-13e-service-transfer.md` — service transfer tests not complete
- `2026-04-27-metadata-fetch-ttl.md` — no metadata-fetch-ttl commits
- `2026-04-27-per-feature-llm-model-knob.md` — no ai-model-per-feature commits
- `2026-04-28-async-clean-1-remove-old-routes.md` — no async-clean-1 commits
- `2026-04-28-async-core-2-dispatcher.md` — no dispatcher implementation
- `2026-04-28-async-core-4-frontend.md` — no async frontend
- `2026-04-28-async-revise-spec-blockers.md` — no spec blockers revision
- `2026-04-28-async-w1-1-fix-read-by-narrator.md` — no read-by-narrator fix
- `2026-04-28-async-w1-3-fix-author-narrator-swap.md` — no author-narrator swap fix
- `2026-04-28-async-w1-4-fix-version-groups.md` — no version groups fix
- `2026-04-28-async-w2-2-cleanup-empty-folders.md` — no empty folders cleanup
- `2026-04-28-async-w2-3-cleanup-organize-mess.md` — no organize cleanup
- `2026-04-28-async-w2-4-fix-library-states.md` — no library state fix
- `2026-04-28-async-w3-3-fix-book-file-paths.md` — no book file path fix
- `2026-04-28-async-w3-4-refetch-missing-authors.md` — no refetch missing authors
- `2026-04-28-async-w3-5-recompute-itunes-paths.md` — no itunes path recompute
- `2026-04-29-relink-2-coauthor-dir.md` — no relink coauthor commits
- `2026-04-29-relink-3-title-normalization.md` — no title normalization commits
- `2026-04-29-relink-manual-fixes.md` — no manual path fixes commits
- `2026-04-29-user-ratings-api.md` — no user ratings API implementation
- `2026-04-30-db-5-time-parse-errors.md` — no time parse errors handling
- `2026-04-30-db-6-pebble-silent-errors.md` — no pebble error handling commits
- `2026-04-30-fe-1-filter-panel.md` — no filter panel refactor
- `2026-04-30-fe-10-coverage.md` — no frontend coverage thresholds set
- `2026-04-30-fe-2-book-grid.md` — no book grid refactor commits
- `2026-04-30-fe-7-console-log.md` — only partial commits
- `2026-04-30-fe-8-error-boundaries.md` — no error boundary implementation
- `2026-04-30-fe-9-localstorage-keys.md` — no localstorage keys cleanup
- `2026-04-30-mock-1-regenerate.md` — no mock regen commits
- `2026-04-30-mock-2-ci-gate.md` — no mock ci-gate implementation
- `2026-04-30-proj-1-summary-columns.md` — no summary columns implementation
- `2026-04-30-proj-2-list-query.md` — no list query implementation
- `2026-04-30-scan-1-walkdir.md` — no scanner walkdir commits
- `2026-04-30-sec-1-browse-allowlist.md` — no browse allowlist implementation
- `2026-05-01-log-5-remaining-printf.md` — no remaining printf removal
- `2026-05-01-r10-fix-capitalized-error-strings.md` — no error string casing fix
- `2026-05-01-r9-remove-stale-todo-comments.md` — no stale todo cleanup
- `2026-05-01-struct-9-frontend-component-splits.md` — no frontend component splits
- `2026-05-01-test-2-fix-database-test-coverage.md` — no database test coverage improvement
- `2026-05-02-dedup-pipeline-p0-01-fingerprint-schema-and-migrations.md` — fingerprint schema missing
- `2026-05-02-dedup-pipeline-p0-02-fingerprint-store-iface.md` — fingerprint store interface not found
- `2026-05-02-dedup-pipeline-p0-05-identity-results-schema.md` — identity results schema not implemented
- `2026-05-02-dedup-pipeline-p0-06-match-groups-schema.md` — match groups schema not found
- `2026-05-02-dedup-pipeline-p1-02-stage-stream-content-hash.md` — stream content hash stage not implemented
- `2026-05-02-dedup-pipeline-p1-03-stage-chromaprint-segments.md` — chromaprint segments stage missing
- `2026-05-02-dedup-pipeline-p1-04-stage-acoustid-lookup.md` — acoustid lookup stage not found
- `2026-05-02-dedup-pipeline-p1-07-stage-embedding-similarity.md` — embedding similarity stage not implemented
- `2026-05-02-dedup-pipeline-p1-08-stage-whisper-intro.md` — whisper intro stage missing
- `2026-05-02-dedup-pipeline-p2-01-decision-matrix-engine.md` — decision matrix engine not found
- `2026-05-02-dedup-pipeline-p2-02-match-group-builder.md` — match group builder not implemented
- `2026-05-02-dedup-pipeline-p3-02-backpressure-and-metrics.md` — backpressure/metrics not implemented
- `2026-05-02-dedup-pipeline-p4-01-identification-endpoints.md` — identification endpoints not found
- `2026-05-02-dedup-pipeline-p4-02-match-groups-v2-endpoints.md` — match groups v2 endpoints missing
- `2026-05-02-dedup-pipeline-p5-01-identification-tab-shell.md` — identification tab not implemented
- `2026-05-02-dedup-pipeline-p5-02-per-book-drawer.md` — per-book drawer component missing
- `2026-05-02-dedup-pipeline-p5-03-match-groups-table.md` — match groups table not found
- `2026-05-02-dedup-pipeline-p6-02-translate-dedup-candidates.md` — dedup candidates translation not done
- `2026-05-02-dedup-pipeline-p6-03-deprecate-legacy-routes.md` — legacy route deprecation not complete
- `2026-05-02-dedup-pipeline-p7-01-trust-ladder-runner.md` — trust ladder runner not implemented
- `2026-05-02-dedup-pipeline-p7-02-admin-opt-in-toggle.md` — admin opt-in toggle not found
- `2026-05-03-unified-book-fingerprint.md` — newly filed (2026-05-03), not yet started
- `2026-05-04-async-embed-batch-api.md` — newly filed (2026-05-04), not yet started
- `2026-05-04-tag-operation-logs.md` — newly filed (2026-05-04), not yet started
- `2026-05-04-broken-files-card-and-repair.md` — newly filed (2026-05-04), not yet started

## Methodology

Frontmatter + "Files" / "Branch" sections were read for each spec.
Symbols and paths were `find` / `grep`-checked against the working
tree, and `git log --oneline --all` was searched for the branch name
or task ID. When evidence was ambiguous or paths had been renamed,
specs were left in NOT_DONE rather than risk a false positive — moving
a still-open task to archive is more harmful than re-classifying a done
one later. Specs filed today (2026-05-03 / 2026-05-04) were
deliberately classified NOT_DONE regardless of any incidental matches.
