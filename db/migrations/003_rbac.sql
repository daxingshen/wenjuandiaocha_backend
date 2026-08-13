-- RBAC 账号权限:两条正交轴之一「角色 role」落地 + 问卷级作答访问模式。
-- role 与套餐 level 正交(level=付费买功能量,role=身份管辖范围),分列两字段不合并。
-- answer_access 决定某问卷谁能作答:anonymous(免登录,现状)/ login_required(需登录,新增)。
-- 设计:wiki/RBAC-账号权限设计.md;流水线:PRD/260813-rbac-account-permission。
-- +goose Up
-- +goose StatementBegin
ALTER TABLE users
  ADD COLUMN role TEXT NOT NULL DEFAULT 'creator';
ALTER TABLE users
  ADD CONSTRAINT users_role_chk CHECK (role IN ('admin', 'creator', 'respondent'));

ALTER TABLE surveys
  ADD COLUMN answer_access TEXT NOT NULL DEFAULT 'anonymous';
ALTER TABLE surveys
  ADD CONSTRAINT surveys_answer_access_chk CHECK (answer_access IN ('anonymous', 'login_required'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE surveys DROP CONSTRAINT surveys_answer_access_chk;
ALTER TABLE surveys DROP COLUMN answer_access;
ALTER TABLE users DROP CONSTRAINT users_role_chk;
ALTER TABLE users DROP COLUMN role;
-- +goose StatementEnd
