-- Create app_tags junction table for many-to-many relationship between apps and tags
CREATE TABLE IF NOT EXISTS app_tags (
    app_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (app_id, tag_id),
    FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_app_tags_app_id ON app_tags(app_id);
CREATE INDEX IF NOT EXISTS idx_app_tags_tag_id ON app_tags(tag_id);
