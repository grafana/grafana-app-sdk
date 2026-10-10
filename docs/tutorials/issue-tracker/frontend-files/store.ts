import { configureStore } from '@reduxjs/toolkit';
import { api as issueAPI } from './generated/issuetrackerproject/v1alpha1/api_gen';

export const store = configureStore({
    reducer: {
        [issueAPI.reducerPath]: issueAPI.reducer,
    },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(issueAPI.middleware),
});
