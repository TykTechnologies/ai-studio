import React from "react";
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import CheckIcon from '@mui/icons-material/Check';
import IconButton from '@mui/material/IconButton';
import './CopyCodeButton.css';

export default function CodeCopyBtn({ children }) {
  const [copyOk, setCopyOk] = React.useState(false);

  const iconColor = copyOk ? '#0af20a' : '#ddd';

  const handleClick = () => {
    navigator.clipboard.writeText(children[0].props.children[0]);
    console.log(children);

    setCopyOk(true);
    setTimeout(() => {
      setCopyOk(false);
    }, 500);
  }

  return (
    <div className="code-copy-btn">
      <IconButton onClick={handleClick} size="small">
        {copyOk ? (
          <CheckIcon style={{ color: iconColor }} />
        ) : (
          <ContentCopyIcon style={{ color: iconColor }} />
        )}
      </IconButton>
    </div>
  )
}