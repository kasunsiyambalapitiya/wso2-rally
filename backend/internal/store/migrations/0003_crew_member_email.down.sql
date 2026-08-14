-- Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
-- Licensed under the Apache License, Version 2.0.
--
-- Reverses 0003. Going back loses every crew member's address, which is what
-- `POST /sessions/join` matches on — so a rollback also needs the pre-0003 join
-- code, or no phone can join.

ALTER TABLE crew_member
  DROP KEY uq_crew_member_email,
  DROP COLUMN email;
