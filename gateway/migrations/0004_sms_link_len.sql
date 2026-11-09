-- 0004_sms_link_len.sql
-- 短信短链列宽调整：short_link 存储含一次性 JWT Token 的完整链接，
-- Token 本体即 ~250 字符（HS256 JWT），基线的 varchar(256) 容纳不下，放宽到 varchar(512)。
-- 幂等（类型相同时重复执行无副作用）。
ALTER TABLE sms_logs ALTER COLUMN short_link TYPE varchar(512);
