-- Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
-- Licensed under the Apache License, Version 2.0.
--
-- Initial WSO2 Motor Rally schema. Every table from the design spec is created
-- here in one migration so no later milestone has to rework it.
--
-- Conventions: ids are 32-char lowercase hex (store.NewID), columns are
-- snake_case, enum string values are snake_case.

CREATE TABLE event (
  id             CHAR(32)     NOT NULL PRIMARY KEY,
  name           VARCHAR(200) NOT NULL,
  event_date     DATE         NOT NULL,
  -- Local wall-clock start, "HH:MM". The 09:00 sync broadcast fires from it.
  start_time     VARCHAR(8)   NOT NULL,
  status         ENUM('setup','active','complete') NOT NULL DEFAULT 'setup',
  start_label    VARCHAR(200) NULL,
  start_lat      DOUBLE       NULL,
  start_lng      DOUBLE       NULL,
  start_radius_m INT          NOT NULL DEFAULT 40,
  end_label      VARCHAR(200) NULL,
  end_lat        DOUBLE       NULL,
  end_lng        DOUBLE       NULL,
  end_radius_m   INT          NOT NULL DEFAULT 30,
  -- Cipher revealed to every crew on the start signal.
  cipher         VARCHAR(200) NULL,
  created_by     VARCHAR(120) NOT NULL,
  created_on     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_event_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE route (
  id            CHAR(32)     NOT NULL PRIMARY KEY,
  event_id      CHAR(32)     NOT NULL,
  name          VARCHAR(120) NOT NULL,
  display_order INT          NOT NULL DEFAULT 0,
  -- CSV import resolves a vehicle's route by name within its event.
  UNIQUE KEY uq_route_name (event_id, name),
  CONSTRAINT fk_route_event FOREIGN KEY (event_id) REFERENCES event (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE waypoint (
  id                CHAR(32)     NOT NULL PRIMARY KEY,
  route_id          CHAR(32)     NOT NULL,
  display_order     INT          NOT NULL DEFAULT 0,
  label             VARCHAR(200) NOT NULL,
  lat               DOUBLE       NOT NULL,
  lng               DOUBLE       NOT NULL,
  boundary_radius_m INT          NOT NULL DEFAULT 50,
  KEY idx_waypoint_route_order (route_id, display_order),
  CONSTRAINT fk_waypoint_route FOREIGN KEY (route_id) REFERENCES route (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE task (
  id       CHAR(32)     NOT NULL PRIMARY KEY,
  event_id CHAR(32)     NOT NULL,
  code     VARCHAR(8)   NOT NULL,
  title    VARCHAR(200) NOT NULL,
  -- taskengine.TaskType, e.g. INPUT_SELECT. Validated in the service layer.
  type     VARCHAR(40)  NOT NULL,
  -- `trigger` is a MySQL reserved word, hence the backticks here and in every
  -- query that touches it.
  `trigger` VARCHAR(20) NOT NULL,
  points   INT          NOT NULL DEFAULT 0,
  sensor   VARCHAR(20)  NOT NULL DEFAULT 'none',
  -- Per-type parameters: cipher options, arithmetic operands, grid solution…
  config   JSON         NOT NULL,
  UNIQUE KEY uq_task_code (event_id, code),
  CONSTRAINT fk_task_event FOREIGN KEY (event_id) REFERENCES event (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE waypoint_task (
  waypoint_id   CHAR(32) NOT NULL,
  task_id       CHAR(32) NOT NULL,
  display_order INT      NOT NULL DEFAULT 0,
  PRIMARY KEY (waypoint_id, task_id),
  KEY idx_waypoint_task_task (task_id),
  CONSTRAINT fk_waypoint_task_waypoint FOREIGN KEY (waypoint_id) REFERENCES waypoint (id) ON DELETE CASCADE,
  CONSTRAINT fk_waypoint_task_task FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE vehicle (
  id             CHAR(32)     NOT NULL PRIMARY KEY,
  event_id       CHAR(32)     NOT NULL,
  code           VARCHAR(20)  NOT NULL,
  team_name      VARCHAR(200) NOT NULL,
  vehicle_type   VARCHAR(40)  NULL,
  contact_number VARCHAR(40)  NULL,
  route_id       CHAR(32)     NULL,
  status         ENUM('ok','breakdown','device_issue') NOT NULL DEFAULT 'ok',
  UNIQUE KEY uq_vehicle_code (event_id, code),
  KEY idx_vehicle_route (route_id),
  CONSTRAINT fk_vehicle_event FOREIGN KEY (event_id) REFERENCES event (id) ON DELETE CASCADE,
  -- Clearing a route leaves its vehicles unassigned rather than deleting them.
  CONSTRAINT fk_vehicle_route FOREIGN KEY (route_id) REFERENCES route (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE crew_member (
  id             CHAR(32)     NOT NULL PRIMARY KEY,
  vehicle_id     CHAR(32)     NOT NULL,
  name           VARCHAR(200) NOT NULL,
  role           ENUM('navigator','node') NOT NULL DEFAULT 'node',
  origin_country VARCHAR(80)  NULL,
  KEY idx_crew_vehicle (vehicle_id),
  CONSTRAINT fk_crew_vehicle FOREIGN KEY (vehicle_id) REFERENCES vehicle (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE team_session (
  id                  CHAR(32)  NOT NULL PRIMARY KEY,
  event_id            CHAR(32)  NOT NULL,
  vehicle_id          CHAR(32)  NOT NULL,
  bound_at            TIMESTAMP NULL,
  started_at          TIMESTAMP NULL,
  finished_at         TIMESTAMP NULL,
  current_waypoint_id CHAR(32)  NULL,
  total_score         INT       NOT NULL DEFAULT 0,
  status              ENUM('bound','active','finished') NOT NULL DEFAULT 'bound',
  -- Last reported position, kept for the organizer live monitor (A6).
  last_lat            DOUBLE    NULL,
  last_lng            DOUBLE    NULL,
  last_ping_at        TIMESTAMP NULL,
  -- One active phone per vehicle: 1 while the session is live, NULL once it is
  -- finished. MySQL treats NULLs as distinct in a unique index, so a vehicle
  -- can hold at most one bound-or-active session while still accumulating any
  -- number of finished ones.
  active_flag         TINYINT GENERATED ALWAYS AS (
                        CASE WHEN status IN ('bound','active') THEN 1 ELSE NULL END
                      ) STORED,
  UNIQUE KEY uq_live_session_per_vehicle (vehicle_id, active_flag),
  KEY idx_session_event_status (event_id, status),
  KEY idx_session_leaderboard (event_id, total_score, finished_at),
  CONSTRAINT fk_session_event FOREIGN KEY (event_id) REFERENCES event (id) ON DELETE CASCADE,
  CONSTRAINT fk_session_vehicle FOREIGN KEY (vehicle_id) REFERENCES vehicle (id) ON DELETE CASCADE,
  CONSTRAINT fk_session_waypoint FOREIGN KEY (current_waypoint_id) REFERENCES waypoint (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE task_submission (
  id             CHAR(32)  NOT NULL PRIMARY KEY,
  session_id     CHAR(32)  NOT NULL,
  task_id        CHAR(32)  NOT NULL,
  waypoint_id    CHAR(32)  NULL,
  status         ENUM('pending','completed','skipped') NOT NULL DEFAULT 'pending',
  payload        JSON      NULL,
  awarded_points INT       NOT NULL DEFAULT 0,
  submitted_at   TIMESTAMP NULL,
  -- A task is scored once per session; resubmission updates this row.
  UNIQUE KEY uq_submission_session_task (session_id, task_id),
  KEY idx_submission_task (task_id),
  CONSTRAINT fk_submission_session FOREIGN KEY (session_id) REFERENCES team_session (id) ON DELETE CASCADE,
  CONSTRAINT fk_submission_task FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE CASCADE,
  CONSTRAINT fk_submission_waypoint FOREIGN KEY (waypoint_id) REFERENCES waypoint (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE vehicle_alert (
  id          CHAR(32)  NOT NULL PRIMARY KEY,
  vehicle_id  CHAR(32)  NOT NULL,
  type        ENUM('breakdown','device_issue','other') NOT NULL,
  note        TEXT      NULL,
  source      ENUM('organizer','crew') NOT NULL DEFAULT 'organizer',
  raised_by   VARCHAR(120) NULL,
  lat         DOUBLE    NULL,
  lng         DOUBLE    NULL,
  raised_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  resolved_at TIMESTAMP NULL,
  -- The dashboard's open-alerts card filters on resolved_at IS NULL.
  KEY idx_alert_vehicle_open (vehicle_id, resolved_at),
  CONSTRAINT fk_alert_vehicle FOREIGN KEY (vehicle_id) REFERENCES vehicle (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE voucher (
  id           CHAR(32)    NOT NULL PRIMARY KEY,
  session_id   CHAR(32)    NOT NULL,
  entry_code   VARCHAR(40) NULL,
  locker_id    VARCHAR(40) NULL,
  lunch_passes INT         NOT NULL DEFAULT 0,
  UNIQUE KEY uq_voucher_session (session_id),
  CONSTRAINT fk_voucher_session FOREIGN KEY (session_id) REFERENCES team_session (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE debrief_video (
  id          CHAR(32)     NOT NULL PRIMARY KEY,
  event_id    CHAR(32)     NOT NULL,
  vehicle_id  CHAR(32)     NULL,
  day         INT          NOT NULL DEFAULT 1,
  object_key  VARCHAR(400) NOT NULL,
  uploaded_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_debrief_event_day (event_id, day),
  CONSTRAINT fk_debrief_event FOREIGN KEY (event_id) REFERENCES event (id) ON DELETE CASCADE,
  CONSTRAINT fk_debrief_vehicle FOREIGN KEY (vehicle_id) REFERENCES vehicle (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
