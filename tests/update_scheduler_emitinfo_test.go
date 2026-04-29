package tests

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestUpdateSchedulerEmitInfo(t *testing.T) {
    repoRoot := filepath.Join("..")
    targetPath := filepath.Join(repoRoot, "internal/server/scheduler.go")

    data, err := os.ReadFile(targetPath)
    if err != nil {
        t.Fatalf("failed to read scheduler.go: %v", err)
    }

    content := string(data)

    replacements := []struct{
        old string
        new string
    }{
        {
            old: "// version: 1.17.2",
            new: "// version: 1.17.3",
        },
        {
            old: "\t\t\treturn ts.triggerOperation(\"series-normalize\", func(ctx context.Context, progress operations.ProgressReporter) error {\n",
            new: "\t\t\treturn ts.triggerOperationWithID(\"series-normalize\", func(ctx context.Context, progress operations.ProgressReporter, opID string) error {\n",
        },
        {
            old: "\t\t\t\t_, err := executeSeriesNormalizeCore(ctx, store, enqueueWB)\n\t\t\t\treturn err",
            new: "\t\t\t\taffected, err := executeSeriesNormalizeCore(ctx, store, enqueueWB)\n" +
                "\t\t\t\tmsg := fmt.Sprintf(\"Series normalize complete: %d series affected, %d books enqueued for write-back\",\n" +
                "\t\t\t\t\tlen(affected), len(affected))\n" +
                "\t\t\t\t_ = progress.Log(\"info\", msg, nil)\n" +
                "\t\t\t\tactivity.EmitInfo(ts.server.activityWriter, opID, \"series-normalize\", \"series-normalize\", msg,\n" +
                "\t\t\t\t\tactivity.TagsIf(len(affected) == 0, activity.NoOpTag)... )\n" +
                "\t\t\t\treturn err",
        },
        {
            old: "\t\t\treturn ts.triggerOperation(\"author-dedup-scan\", func(ctx context.Context, progress operations.ProgressReporter) error {\n",
            new: "\t\t\treturn ts.triggerOperationWithID(\"author-dedup-scan\", func(ctx context.Context, progress operations.ProgressReporter, opID string) error {\n",
        },
        {
            old: "\t\t\t\tresultMsg := fmt.Sprintf(\"Dedup scan complete: %d duplicate groups found across %d authors\", len(groups), total)\n" +
                "\t\t\t\t_ = progress.Log(\"info\", resultMsg, nil)\n" +
                "\t\t\t\t_ = progress.UpdateProgress(100, 100, resultMsg)\n" +
                "\t\t\t\treturn nil",
            new: "\t\t\t\tresultMsg := fmt.Sprintf(\"Dedup scan complete: %d duplicate groups found across %d authors\", len(groups), total)\n" +
                "\t\t\t\t_ = progress.Log(\"info\", resultMsg, nil)\n" +
                "\t\t\t\t_ = progress.UpdateProgress(100, 100, resultMsg)\n" +
                "\t\t\t\tactivity.EmitInfo(ts.server.activityWriter, opID, \"author-dedup-scan\", \"author-dedup-scan\", resultMsg,\n" +
                "\t\t\t\t\tactivity.TagsIf(len(groups) == 0, activity.NoOpTag)... )\n" +
                "\t\t\t\treturn nil",
        },
    }

    for _, rep := range replacements {
        if !strings.Contains(content, rep.old) {
            t.Fatalf("expected to find snippet %q", rep.old)
        }
        content = strings.Replace(content, rep.old, rep.new, 1)
    }

    if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
        t.Fatalf("failed to write scheduler.go: %v", err)
    }
}
