import React from 'react';
import { Typography } from '@mui/material';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { a11yDark } from 'react-syntax-highlighter/dist/cjs/styles/prism';
import CodeCopyBtn from '../CopyCodeButton';

const MarkdownMessage = ({ content }) => {
	const inlineCodeStyle = {
		padding: '2px 4px',
		color: '#232629',
		backgroundColor: 'rgb(240, 240, 240)',
		borderRadius: '3px',
		fontFamily: 'monospace',
		fontSize: '1rem',
	};

	const Pre = ({ children }) => (
		<pre className="code-pre">
			<CodeCopyBtn>{children}</CodeCopyBtn>
			{children}
		</pre>
	);

	return (
		<ReactMarkdown
			components={{
				p: ({ node, ...props }) => <Typography sx={{ fontSize: '1rem' }} component="div" {...props} />,
				a: ({ node, ...props }) => (
					<a target="_blank" rel="noopener noreferrer" {...props} />
				),
				pre: Pre,
				code: ({ node, inline, className, children, ...props }) => {
					const match = /language-(\w+)/.exec(className || '');
					const language = match ? match[1] : 'text'; // Default to 'text' for unknown/unspecified languages

					if (inline) {
						return (
							<code style={inlineCodeStyle} {...props}>
								{children}
							</code>
						);
					}

					// Always use SyntaxHighlighter for code blocks
					return (
						<SyntaxHighlighter
							style={a11yDark}
							language={language}
							PreTag="div"
							showLineNumbers={!inline && language !== 'text'} // Show line numbers except for plain text
							{...props}
						>
							{String(children).replace(/\n$/, '')}
						</SyntaxHighlighter>
					);
				},
			}}
			remarkPlugins={[remarkGfm]}
		>
			{content}
		</ReactMarkdown>
	);
};

export default MarkdownMessage;
