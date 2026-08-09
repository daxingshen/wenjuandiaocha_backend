-- 问卷状态机约束:status 仅允许 draft|live|closed(生命周期 draft→live→closed→live)。
-- 001 里 status 只有注释约束,无 DB 层强制;本迁移补 CHECK,防止非法值落库。
-- +goose Up
-- +goose StatementBegin
ALTER TABLE surveys
  ADD CONSTRAINT surveys_status_chk CHECK (status IN ('draft', 'live', 'closed'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE surveys
  DROP CONSTRAINT surveys_status_chk;
-- +goose StatementEnd
