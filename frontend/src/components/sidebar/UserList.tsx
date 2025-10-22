/**
 * Active users list component
 */

import { Heading, Stack } from '@chakra-ui/react';
import User from '../shared/User';
import type { UserInfo } from '../../types';

export interface UserListProps {
  currentUser: UserInfo & { id: number };
  users: Record<number, UserInfo>;
  darkMode: boolean;
  onChangeName: (name: string) => void;
  onChangeColor: () => void;
}

export function UserList({
  currentUser,
  users,
  darkMode,
  onChangeName,
  onChangeColor,
}: UserListProps) {
  return (
    <>
      <Heading mt={4} mb={1.5} size="sm">
        Active Users
      </Heading>
      <Stack spacing={0} mb={1.5} fontSize="sm">
        <User
          info={currentUser}
          isMe
          onChangeName={onChangeName}
          onChangeColor={onChangeColor}
          darkMode={darkMode}
        />
        {Object.entries(users).map(([id, info]) => (
          <User key={id} info={info} darkMode={darkMode} />
        ))}
      </Stack>
    </>
  );
}
