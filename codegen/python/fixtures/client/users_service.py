class UsersService:
    def __init__(self, client):
        self.client = client



    def get_users(self, headers=None, query_params=None):
        """
        First line of comment.
        Second line of comment
        It is method for GET /users
        """
        uri = self.client.base_url + "/users"
        return self.client.session.get(uri, headers=headers, params=query_params)


    def create_users(self, data, headers=None, query_params=None):
        """
        create users
        It is method for POST /users
        """
        uri = self.client.base_url + "/users"
        return self.client.post(uri, data, headers=headers, params=query_params)


    def option_users(self, headers=None, query_params=None):
        """
        It is method for OPTIONS /users
        """
        uri = self.client.base_url + "/users"
        return self.client.session.options(uri, headers=headers, params=query_params)


    def getuserid(self, userId, headers=None, query_params=None):
        """
        get id
        It is method for GET /users/{userId}
        """
        uri = self.client.base_url + "/users/"+userId
        return self.client.session.get(uri, headers=headers, params=query_params)


    def users_byUserId_delete(self, userId, headers=None, query_params=None):
        """
        It is method for DELETE /users/{userId}
        """
        uri = self.client.base_url + "/users/"+userId
        return self.client.session.delete(uri, headers=headers, params=query_params)


    def users_byUserId_address_byAddressId_get(self, addressId, userId, headers=None, query_params=None):
        """
        get address id
        of address
        It is method for GET /users/{userId}/address/{addressId}
        """
        uri = self.client.base_url + "/users/"+userId+"/address/"+addressId
        return self.client.session.get(uri, headers=headers, params=query_params)
