import React from "react";
import {
  Table,
  TableBody,
  TableHead,
  TableCell,
  Typography,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import CloseIcon from "@mui/icons-material/Close";
import {
  StyledTableCell,
  StyledTableRow,
} from "../../../styles/sharedStyles";
import {
  AddButton,
  RemoveButton,
  TableHeaderRow,
} from "./styles";

const TransferListTable = ({
  items = [],
  columns = [],
  idField = "id",
  isLeftSide,
  onAddItem,
  onRemoveItem,
}) => {
  return (
    <Table style={{ width: "100%", tableLayout: "fixed" }}>
      <TableHead>
        <TableHeaderRow>
          {columns.map((column) => (
            <TableCell
              key={column.field}
              width={column.width}
              style={{
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
            >
              <Typography variant="bodyMediumSemiBold" color="text.defaultSubdued">
                {column.headerName}
              </Typography>
            </TableCell>
          ))}
          <TableCell
            align="right"
            width="25%"
            style={{ whiteSpace: "nowrap" }}
          >
            <Typography variant="bodyMediumSemiBold" color="text.defaultSubdued">
              Actions
            </Typography>
          </TableCell>
        </TableHeaderRow>
      </TableHead>
      <TableBody>
        {items.length > 0 ? (
          items.map((item) => (
            <StyledTableRow key={item[idField]}>
              {columns.map((column) => (
                <StyledTableCell
                  key={`${item[idField]}-${column.field}`}
                  width={column.width}
                  style={{
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                  }}
                >
                  <div
                    style={{
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      wordBreak: "break-word",
                      display: "-webkit-box",
                      WebkitLineClamp: 3,
                      WebkitBoxOrient: "vertical",
                      maxHeight: "4.5em",
                      lineHeight: "1.5em",
                      width: "100%",
                    }}
                  >
                    {column.renderCell
                      ? column.renderCell(item)
                      : item[column.field]}
                  </div>
                </StyledTableCell>
              ))}
              <StyledTableCell
                align="right"
                width="10%"
                style={{ whiteSpace: "nowrap" }}
              >
                {isLeftSide ? (
                  <RemoveButton onClick={() => onRemoveItem(item)}>
                    <CloseIcon />
                  </RemoveButton>
                ) : (
                  <AddButton onClick={() => onAddItem(item)}>
                    <AddIcon />
                  </AddButton>
                )}
              </StyledTableCell>
            </StyledTableRow>
          ))
        ) : (
          <StyledTableRow>
            <StyledTableCell colSpan={columns.length + 1} align="center">
              <Typography color="text.defaultSubdued">
                No items to display
              </Typography>
            </StyledTableCell>
          </StyledTableRow>
        )}
      </TableBody>
    </Table>
  );
};

export default TransferListTable;