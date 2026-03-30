import { useMemo, useCallback } from 'react';
import { format } from 'date-fns';

const usePrintChat = ({ chatName, messages, messagesContainerRef }) => {
  const canPrint = useMemo(() => {
    return messages.some(m => m.type === 'user' || m.role === 'user');
  }, [messages]);

  const handlePrint = useCallback(() => {
    if (!canPrint) return;

    // Create and inject print header
    const header = document.createElement('div');
    header.setAttribute('data-print-role', 'print-header');
    header.style.display = 'none'; // Hidden on screen, shown via print CSS
    header.innerHTML = `
      <h1>${chatName || 'Chat Conversation'}</h1>
      <p>Exported on ${format(new Date(), 'MMMM d, yyyy \'at\' h:mm a')}</p>
    `;

    // Insert header at the top of the messages container
    const container = messagesContainerRef?.current;
    if (container) {
      container.insertBefore(header, container.firstChild);
    } else {
      document.body.insertBefore(header, document.body.firstChild);
    }

    const cleanup = () => {
      header.remove();
    };

    window.addEventListener('afterprint', cleanup, { once: true });

    // Fallback cleanup in case afterprint doesn't fire
    const fallbackTimeout = setTimeout(cleanup, 5000);
    window.addEventListener('afterprint', () => clearTimeout(fallbackTimeout), { once: true });

    window.print();
  }, [canPrint, chatName, messagesContainerRef]);

  return { handlePrint, canPrint };
};

export default usePrintChat;
