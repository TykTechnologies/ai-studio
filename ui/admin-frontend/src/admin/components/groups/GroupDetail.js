import { Typography, CircularProgress, Box, Chip } from "@mui/material";
import { TitleBox, ContentBox, SecondaryLinkButton } from "../../styles/sharedStyles";
import Section from "../common/Section";
import CollapsibleSection from "../common/CollapsibleSection";
import TeamMembersTable from "./components/TeamMembersTable";
import { useNavigate } from "react-router-dom";
import useGroupDetail from "./hooks/useGroupDetail";
import { CATALOG_ROWS, borderStyle, lastRowStyle, TEAM_MEMBERS_COLUMNS } from "./utils/groupDetailConfig";

const GroupDetail = () => {
  const navigate = useNavigate();
  const {
    group,
    users,
    catalogues,
    dataCatalogues,
    toolCatalogues,
    loading,
    error,
  } = useGroupDetail();

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;
  if (!group) return <Typography>Group not found</Typography>;

  const userRows = users.map((u) => ({
    id: u.id,
    name: u.attributes?.name,
    email: u.attributes?.email,
    role: u.attributes?.role,
  }));

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">User group details</Typography>
        <SecondaryLinkButton
          color="inherit"
          onClick={() => navigate("/admin/groups")}
        >
          Back to user groups
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        {/* Section 1: Name */}
        <Section>
          <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
            <Typography variant="bodyLargeBold" color="text.primary">
              Name
            </Typography>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {group.attributes?.name}
            </Typography>
          </div>
        </Section>

        {/* Section 2: Team Members */}
        <CollapsibleSection title="Team members" defaultExpanded>
          <TeamMembersTable columns={TEAM_MEMBERS_COLUMNS} rows={userRows} />
        </CollapsibleSection>

        {/* Section 3: Catalogs */}
        <CollapsibleSection title="Catalogs" defaultExpanded>
          {CATALOG_ROWS.map((row, idx) => {
            const items =
              row.itemsKey === "catalogues"
                ? catalogues
                : row.itemsKey === "dataCatalogues"
                ? dataCatalogues
                : toolCatalogues;

            return (
              <Box
                key={row.label}
                sx={idx < CATALOG_ROWS.length - 1 ? borderStyle : lastRowStyle}
              >
                <Typography
                  variant="bodyLargeBold"
                  color="text.primary"
                  sx={{ minWidth: 140 }}
                >
                  {row.label}
                </Typography>
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                  {items && items.length > 0 ? (
                    items.map((item) => (
                      <Chip
                        key={item.id}
                        label={item.attributes?.name}
                        size="small"
                        sx={{
                          fontWeight: 500,
                          fontSize: "1rem",
                          background: "background.neutralDefault",
                          color: "text.primary",
                          borderRadius: "8px",
                          height: "28px",
                        }}
                      />
                    ))
                  ) : (
                    <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                      None
                    </Typography>
                  )}
                </Box>
              </Box>
            );
          })}
        </CollapsibleSection>
      </ContentBox>
    </>
  );
};

export default GroupDetail;
