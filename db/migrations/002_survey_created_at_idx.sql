-- +goose Up
-- +goose StatementBegin

-- 问卷列表 keyset 游标翻页:排序键 created_at DESC(id 兜底保全序)。
-- 加 created_at 索引支撑范围扫描;id 已是主键,同 created_at 边界内的 id 排序由主键兜底,
-- 数据量下无需复合索引(见 wiki PRD 03c 决策)。
CREATE INDEX idx_surveys_created_at ON surveys(created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_surveys_created_at;

-- +goose StatementEnd
