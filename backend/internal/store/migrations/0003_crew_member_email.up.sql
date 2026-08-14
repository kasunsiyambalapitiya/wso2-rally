-- Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
-- Licensed under the Apache License, Version 2.0.
--
-- Gives every crew member the address the super app authenticates them by.
--
-- The in-car app is embedded in the WSO2 Open Super App, so a joining phone
-- already carries an Asgardeo token naming the person holding it. The roster no
-- longer has to establish *who* someone is — only which car they are in — so
-- `POST /sessions/join` matches this column against the token's email claim and
-- the last-four-digits check is gone.
--
-- NOT NULL for the reason `phone_number` used to be: a member with no address
-- could never join, and a roster that looks provisioned but strands someone at
-- the start line is worse than one that refuses to save.
--
-- Unique per vehicle rather than globally: one person may legitimately appear on
-- two different events' rosters, but never twice in the same car.
--
-- No backfill: no event has run, so there are no rows to preserve. A deployment
-- that somehow has crew rows gets the empty-string default, which matches
-- nothing and therefore denies rather than admits.

ALTER TABLE crew_member
  ADD COLUMN email VARCHAR(320) NOT NULL DEFAULT '' AFTER name,
  ADD UNIQUE KEY uq_crew_member_email (vehicle_id, email);
