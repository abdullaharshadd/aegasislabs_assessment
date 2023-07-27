import requests

BASE_URL = "http://127.0.0.1:5000"  # Replace this with the base URL of your running API server

def create_prompt(prompt):
    endpoint = "/create"
    data = {"prompt": prompt}
    response = requests.post(BASE_URL + endpoint, json=data)
    return response.json()

def get_response(prompt_index):
    endpoint = f"/get/{prompt_index}"
    response = requests.get(BASE_URL + endpoint)

    try:
        return response.json()
    except requests.exceptions.JSONDecodeError:
        return {"error": "Invalid response from the server"}

def update_prompt(prompt_index, new_prompt):
    endpoint = f"/update/{prompt_index}"
    data = {"new_prompt": new_prompt}
    response = requests.put(BASE_URL + endpoint, json=data)
    return response.json()

def delete_prompt(prompt_index):
    endpoint = f"/delete/{prompt_index}"
    response = requests.delete(BASE_URL + endpoint)

    try:
        return response.json()
    except requests.exceptions.JSONDecodeError:
        return {"error": "Invalid response from the server"}

def main():
    # Test create_prompt
    prompt1 = "What is life?"
    prompt2 = "What is the capital of Pakistan?"
    create_response1 = create_prompt(prompt1)
    create_response2 = create_prompt(prompt2)

    print(create_response1)
    print(create_response2)

    # Test get_response
    prompt_index = 0
    response = get_response(prompt_index)
    print(response)

    # Test update_prompt
    prompt_index = 1
    new_prompt = "Who is Goku?"
    update_response = update_prompt(prompt_index, new_prompt)
    print(update_response)

    # Test get_response after update
    response_after_update = get_response(prompt_index)
    print(response_after_update)

    # Test delete_prompt
    prompt_index = 0
    delete_response = delete_prompt(prompt_index)
    print(delete_response)

    # Test get_response after delete
    response_after_delete = get_response(prompt_index)
    print(response_after_delete)

if __name__ == "__main__":
    main()
