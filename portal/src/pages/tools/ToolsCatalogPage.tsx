import React, { useEffect, useState } from 'react';

interface Tool {
  id: string;
  name: string;
  description: string;
}

const ToolsCatalogPage: React.FC = () => {
  const [tools, setTools] = useState<Tool[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchTools = async () => {
      try {
        // Replace with actual API call
        const response = await fetch('/api/tools');
        if (!response.ok) {
          throw new Error('Failed to fetch tools');
        }
        const data = await response.json();
        setTools(data);
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    fetchTools();
  }, []);

  if (loading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error: {error}</div>;
  }

  return (
    <div>
      <h1>Tools Catalog</h1>
      {tools.length === 0 ? (
        <p>No tools available.</p>
      ) : (
        <ul>
          {tools.map((tool) => (
            <li key={tool.id}>
              <h2>{tool.name}</h2>
              <p>{tool.description}</p>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

export default ToolsCatalogPage;
