import React from "react";
import { Routes, Route } from "react-router-dom";
import adminRoutes from "../admin/routes";

const AdminRoutes = () => <Routes>{adminRoutes}</Routes>;

export default AdminRoutes;
