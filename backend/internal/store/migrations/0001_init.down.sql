-- Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
-- Licensed under the Apache License, Version 2.0.
--
-- Reverses 0001_init. Tables drop in reverse dependency order so no foreign
-- key ever references a missing parent.

DROP TABLE IF EXISTS debrief_video;
DROP TABLE IF EXISTS voucher;
DROP TABLE IF EXISTS vehicle_alert;
DROP TABLE IF EXISTS task_submission;
DROP TABLE IF EXISTS session_waypoint_visit;
DROP TABLE IF EXISTS session_device;
DROP TABLE IF EXISTS team_session;
DROP TABLE IF EXISTS crew_member;
DROP TABLE IF EXISTS vehicle;
DROP TABLE IF EXISTS waypoint_task;
DROP TABLE IF EXISTS task;
DROP TABLE IF EXISTS waypoint;
DROP TABLE IF EXISTS route;
DROP TABLE IF EXISTS event;
