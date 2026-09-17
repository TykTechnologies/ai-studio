import React, { useEffect, useState } from "react";
import { Alert, Snackbar } from "@mui/material";
import { subscribeListTruncated } from "../../admin/utils/listAll";

/**
 * One snackbar for every picker or lookup list that listAll had to cap,
 * mounted once in the layout, so a missing option is never silent.
 */
const ListTruncatedToaster = () => {
  const [detail, setDetail] = useState(null);

  useEffect(() => subscribeListTruncated(setDetail), []);

  return (
    <Snackbar
      open={Boolean(detail)}
      autoHideDuration={8000}
      onClose={() => setDetail(null)}
      anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
    >
      <Alert severity="warning" onClose={() => setDetail(null)} data-testid="list-truncated-toast">
        {`A list on this page has more than ${detail?.limit} entries; only the first ${detail?.limit} are shown.`}
      </Alert>
    </Snackbar>
  );
};

export default ListTruncatedToaster;
