CREATE TABLE `todos` (
  `id` uuid PRIMARY KEY,
  `task` varchar(255) NOT NULL,
  `completed` bool DEFAULT false,
  `notes` text,
  `due` datetime,
  `priority` bool,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL
);