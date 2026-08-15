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
-- Four statements, not one, and the order matters.
--
-- Adding the column with a constant default *and* the unique index in one
-- statement fails on any table that already holds two crew members for one
-- vehicle: both rows take the same default and collide. So the column arrives
-- with a temporary default, the backfill gives every existing row a value unique
-- by construction (its own id) in the reserved `.invalid` TLD (RFC 2606) that no
-- identity provider can ever issue — such a row denies a join rather than
-- admitting the wrong person, and reads as obviously unfinished in the
-- organizer's crew editor — and only then is the index added.
--
-- The default is dropped at the end on purpose. Left in place, an INSERT that
-- forgot the column would silently store '' and fail much later, on the *second*
-- member of some vehicle, as a duplicate-key error naming an empty string. With
-- no default it fails immediately and says which column is missing.

ALTER TABLE crew_member
  ADD COLUMN email VARCHAR(320) NOT NULL DEFAULT '' AFTER name;

UPDATE crew_member
SET email = CONCAT('unset-', id, '@invalid')
WHERE email = '';

ALTER TABLE crew_member
  ADD UNIQUE KEY uq_crew_member_email (vehicle_id, email);

ALTER TABLE crew_member
  ALTER COLUMN email DROP DEFAULT;
