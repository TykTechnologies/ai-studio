import { Typography, CircularProgress, Box, Chip } from "@mui/material";
import { TitleBox, ContentBox, SecondaryLinkButton, ResponsiveTitleBox, TitleContentBox, ActionButtonsBox, PrimaryButton } from "../../styles/sharedStyles";
import Section from "../common/Section";
import CollapsibleSection from "../common/CollapsibleSection";
import TeamMembersTable from "./components/TeamMembersTable";
import { useNavigate, Link } from "react-router-dom";
import useGroupDetail from "./hooks/useGroupDetail";
import { CATALOG_ROWS, borderStyle, lastRowStyle, TEAM_MEMBERS_COLUMNS_FOR_TABLE } from "./utils/groupDetailConfig";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import EditIcon from "@mui/icons-material/Edit";
import useTeamMembers from "./hooks/useTeamMembers";

const GroupDetail = () => {
  const navigate = useNavigate();
  const {
    group,
    catalogues,
    dataCatalogues,
    toolCatalogues,
    loading: groupLoading,
    error: groupError,
  } = useGroupDetail();

  const {
    users,
    error: usersError,
    loading: usersLoading,
    isLoadingMore,
    hasMore,
    handleLoadMore,
    containerRef,
  } = useTeamMembers(group?.id);

  const loading = groupLoading;
  const error = groupError || usersError;

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;
  if (!group) return <Typography>Group not found</Typography>;

  const userRows = users?.map((u) => ({
    id: u.id,
    name: u.attributes?.name,
    email: u.attributes?.email,
    role: u.attributes?.role,
  }));

  return (
    <>
      <TitleBox>
        <ResponsiveTitleBox>
          <TitleContentBox>
            <SecondaryLinkButton
              component={Link}
              to="/admin/groups"
              color="inherit"
              sx={{ mb: 1, px: 0 }}
              startIcon={<ChevronLeftIcon sx={{ mr: -1 }} />}
            >
              back to teams
            </SecondaryLinkButton>
            <Typography variant="headingXLarge">
              Team details
            </Typography>
          </TitleContentBox>
          <ActionButtonsBox>
            <PrimaryButton
              variant="contained"
              startIcon={<EditIcon />}
              onClick={() => navigate(`/admin/groups/edit/${group.id}`)}
            >
              Edit team
            </PrimaryButton>
          </ActionButtonsBox>
        </ResponsiveTitleBox>
      </TitleBox>
      <ContentBox sx={{
        maxWidth: {
          xs: '100%',
          sm: '100%',
          md: '100%',
          lg: '75%'
        }
      }}>
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
        <TeamMembersTable 
          columns={TEAM_MEMBERS_COLUMNS_FOR_TABLE} 
          rows={userRows} 
          loading={usersLoading}
          isLoadingMore={isLoadingMore}
          hasMore={hasMore}
          onLoadMore={handleLoadMore}
          containerRef={containerRef}
        />
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
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, justifyContent: "flex-start" }}>
                  {items && items?.length > 0 ? (
                    items.map((item) => (
                      <Chip
                        key={item.id}
                        label={item.attributes?.name}
                        size="small"
                        sx={{
                          bgcolor: theme => theme.palette.background.buttonPrimaryOutlineHover,
                          borderRadius: '6px',
                          maxHeight: 'fit-content',
                          '& .MuiChip-label': {
                            color: theme => theme.palette.text.defaultSubdued,
                            padding: '2px 6px',
                            marginRight: '6px',
                          },
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
