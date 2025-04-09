import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Link } from 'react-router-dom';

/**
 * NotificationMarkdown component for rendering markdown content in notifications
 * with special handling for internal links
 */
const NotificationMarkdown = ({ children }) => {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        a: ({ node, ...props }) => {
          // Check if this is an internal link
          if (props.href && props.href.startsWith('/')) {
            // Use React Router's Link component for internal navigation
            return (
              <Link 
                to={props.href}
              >
                {props.children}
              </Link>
            );
          }
          
          // For external links, use regular anchor tags with target="_blank"
          return (
            <a 
              href={props.href}
              target="_blank"
              rel="noopener noreferrer"
            >
              {props.children}
            </a>
          );
        }
      }}
    >
      {children}
    </ReactMarkdown>
  );
};

export default NotificationMarkdown;
