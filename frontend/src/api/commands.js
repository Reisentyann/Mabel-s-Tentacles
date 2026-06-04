import api from './index';

export const getCommands = (page = 1, size = 10) => {
  return api.get(`/commands/?page=${page}&size=${size}`);
};

export const getCommandDetail = (id) => {
  return api.get(`/commands/${id}`);
};