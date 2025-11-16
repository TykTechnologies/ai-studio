import React, { createContext, useContext, useState, useEffect } from 'react';
import axios from 'axios';

const EditionContext = createContext();

export const EditionProvider = ({ children }) => {
	const [edition, setEdition] = useState('community'); // default to community
	const [version, setVersion] = useState('');
	const [isEnterprise, setIsEnterprise] = useState(false);
	const [loading, setLoading] = useState(true);

	useEffect(() => {
		const fetchEditionInfo = async () => {
			try {
				const response = await axios.get('/common/system');
				const editionValue = response.data.edition || 'community';
				const versionValue = response.data.version || '';

				setEdition(editionValue);
				setVersion(versionValue);
				setIsEnterprise(editionValue === 'enterprise');
			} catch (error) {
				console.error('Error fetching edition info:', error);
				// Default to community edition on error
				setEdition('community');
				setIsEnterprise(false);
			} finally {
				setLoading(false);
			}
		};

		fetchEditionInfo();
	}, []);

	return (
		<EditionContext.Provider value={{ edition, version, isEnterprise, loading }}>
			{children}
		</EditionContext.Provider>
	);
};

export const useEdition = () => {
	const context = useContext(EditionContext);
	if (!context) {
		throw new Error('useEdition must be used within an EditionProvider');
	}
	return context;
};
