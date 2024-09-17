import React, { useState, useEffect } from 'react';
import apiClient from './apiClient';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  CircularProgress,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';

const UserDetails = ({ user, open, onClose, onUserUpdate }) => {
  const [userGroups, setUserGroups] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (user) {
      fetchUserGroups();
    }
  }, [user]);

  const fetchUserGroups = async () => {
    try {
      const response = await apiClient.get(`/users/${user.id}/groups`);
      setUserGroups(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error('Error fetching user groups', error);
      setLoading(false);
    }
  };

  const handleRemoveFromGroup = async (groupId) => {
    try {
      await apiClient.delete(`/groups/${groupId}/users/${user.id}`);
      await fetchUserGroups();
      onUserUpdate();
    } catch (error) {
      console.error('Error removing user from group', error);
    }
  };

  if (!user) return null;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>User Details</DialogTitle>
      <DialogContent>
        <Typography variant="h6">{user.attributes.name}</Typography>
        <Typography variant="body1">{user.attributes.email}</Typography>
        <Typography variant="h6" style={{ marginTop: '1rem' }}>Groups</Typography>
        {loading ? (
          <CircularProgress />
        ) : (
          <List>
            {userGroups.length > 0 ? (
              userGroups.map((group) => (
                <ListItem key={group.id}>
                  <ListItemText primary={group.attributes.name} />
                  <ListItemSecondaryAction>
                    <IconButton edge="end" aria-label="delete" onClick={() => handleRemoveFromGroup(group.id)}>
                      <DeleteIcon />
                    </IconButton>
                  </ListItemSecondaryAction>
                </ListItem>
              ))
            ) : (
              <ListItem>
                <ListItemText primary="User is not a member of any groups" />
              </ListItem>
            )}
          </List>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} color="primary">Close</Button>
      </DialogActions>
    </Dialog>
  );
};

export default UserDetails;
