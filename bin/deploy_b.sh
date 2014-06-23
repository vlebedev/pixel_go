#!/bin/bash
ansible-playbook -i hosts -l prod deploy.yml --extra-vars "node_name=b"
