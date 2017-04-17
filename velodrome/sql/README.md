Overview
========

This defines the model that `Fetcher` uses to save the data in the database, and
that `Transform` reads in order to generate the metrics.

The database and the tables are automatically created on the first connection by
[Gorm](https://github.com/jinzhu/gorm). The mapping between the Go objects here
and the database rows is also done by Gorm.

Schema
======

Here is what the final schema looks like:

List of tables in the `github` database:
```
+------------------+
| Tables_in_github |
+------------------+
| comments         |
| issue_events     |
| issues           |
| labels           |
+------------------+
```

`comments` table:
```
+--------------------+--------------+------+-----+---------+----------------+
| Field              | Type         | Null | Key | Default | Extra          |
+--------------------+--------------+------+-----+---------+----------------+
| id                 | int(11)      | NO   | PRI | NULL    | auto_increment |
| issue_id           | int(11)      | YES  |     | NULL    |                |
| body               | text         | YES  |     | NULL    |                |
| user               | varchar(255) | YES  |     | NULL    |                |
| comment_created_at | timestamp    | YES  |     | NULL    |                |
| comment_updated_at | timestamp    | YES  |     | NULL    |                |
| pull_request       | tinyint(1)   | YES  |     | NULL    |                |
+--------------------+--------------+------+-----+---------+----------------+
```

`issue_events` table:
```
+------------------+--------------+------+-----+---------+----------------+
| Field            | Type         | Null | Key | Default | Extra          |
+------------------+--------------+------+-----+---------+----------------+
| id               | int(11)      | NO   | PRI | NULL    | auto_increment |
| label            | varchar(255) | YES  |     | NULL    |                |
| event            | varchar(255) | YES  |     | NULL    |                |
| event_created_at | timestamp    | YES  |     | NULL    |                |
| issue_id         | int(11)      | YES  |     | NULL    |                |
| assignee_id      | varchar(255) | YES  |     | NULL    |                |
| actor_id         | varchar(255) | YES  |     | NULL    |                |
| assignee         | varchar(255) | YES  |     | NULL    |                |
| actor            | varchar(255) | YES  |     | NULL    |                |
+------------------+--------------+------+-----+---------+----------------+
```

`issues` table:
```
+------------------+---------------+------+-----+---------+----------------+
| Field            | Type          | Null | Key | Default | Extra          |
+------------------+---------------+------+-----+---------+----------------+
| id               | int(11)       | NO   | PRI | NULL    | auto_increment |
| title            | varchar(1000) | YES  |     | NULL    |                |
| body             | text          | YES  |     | NULL    |                |
| user             | varchar(255)  | YES  |     | NULL    |                |
| assignee         | varchar(255)  | YES  |     | NULL    |                |
| state            | varchar(255)  | YES  |     | NULL    |                |
| comments         | int(11)       | YES  |     | NULL    |                |
| is_pr            | tinyint(1)    | YES  |     | NULL    |                |
| issue_closed_at  | timestamp     | YES  |     | NULL    |                |
| issue_created_at | timestamp     | YES  |     | NULL    |                |
| issue_updated_at | timestamp     | YES  |     | NULL    |                |
+------------------+---------------+------+-----+---------+----------------+
```

And `labels` table:
```
+----------+--------------+------+-----+---------+-------+
| Field    | Type         | Null | Key | Default | Extra |
+----------+--------------+------+-----+---------+-------+
| issue_id | int(11)      | YES  | MUL | NULL    |       |
| name     | varchar(255) | YES  |     | NULL    |       |
+----------+--------------+------+-----+---------+-------+
```

And `assignees` table:
```
+------------+--------------+------+-----+---------+-------+
| Field      | Type         | Null | Key | Default | Extra |
+------------+--------------+------+-----+---------+-------+
| repository | varchar(255) | YES  |     | NULL    |       |
| issue_id   | varchar(255) | YES  |     | NULL    |       |
| name       | varchar(255) | YES  |     | NULL    |       |
+------------+--------------+------+-----+---------+-------+
```
